package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const maxCommits = 100

var minDate = time.Now().AddDate(0, -18, 0)

type resultOrError struct {
	commits  []*object.Commit
	repoName string
	err      error
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("too few arguments. Please specify a regex and a directory containing git repos.")
		os.Exit(1)
	}
	regexString := os.Args[1]
	regex, err := regexp.Compile(regexString)
	if err != nil {
		fmt.Printf("Failed to compile regex %q", regexString)
		os.Exit(1)
	}

	err = SearchLog(regex, os.Args[2])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
func SearchLog(regex *regexp.Regexp, dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	resultChannel := make(chan resultOrError)
	numGoroutines := 0
	for _, f := range files {
		if f.IsDir() {
			path := filepath.Join(dir, f.Name())
			repo, err := git.PlainOpen(path)
			if err != nil {
				fmt.Printf("%s %v", f.Name(), err)
				continue
			}
			go func(regex *regexp.Regexp, repo *git.Repository, resultChannel chan resultOrError, repoName string) {
				resultChannel <- searchLogInRepo(regex, repo, repoName)
			}(regex, repo, resultChannel, f.Name())
			numGoroutines++
		}
	}
	fmt.Println("All goroutines got started")
	resultsCollected := 0
	for result := range resultChannel {
		resultsCollected++
		if result.err != nil {
			return err
		}
		for _, commit := range result.commits {
			printCommit(commit, result.repoName)
		}
		if resultsCollected == numGoroutines {
			break
		}
	}
	return nil
}

type stopIterError struct{}

func (e stopIterError) Error() string {
	return "stop"
}

func searchLogInRepo(regex *regexp.Regexp, repo *git.Repository, repoName string) resultOrError {
	options := git.LogOptions{Order: git.LogOrderCommitterTime}
	cIter, err := repo.Log(&options)
	if err != nil {
		return resultOrError{nil, repoName, err}
	}
	var foundCommits []*object.Commit

	err = cIter.ForEach(func(commit *object.Commit) error {
		//fmt.Printf("%v len(foundCommits): %d\n", commit.Author.When, len(foundCommits))
		switch {
		case len(commit.ParentHashes) == 0:
			return nil
		case len(commit.ParentHashes) == 1:
			parentCommit, err := repo.CommitObject(commit.ParentHashes[0])
			if err != nil {
				return err
			}
			foundCommits, err = checkDiff(regex, parentCommit, commit, foundCommits)
			if err != nil {
				return err
			}
			if len(foundCommits) >= maxCommits {
				return stopIterError{}
			}
			if commit.Author.When.Before(minDate) {
				return stopIterError{}
			}
		}
		return nil
	})
	_, ok := err.(stopIterError)
	if ok || err == nil {
		// everything is fine, return found commits
		return resultOrError{foundCommits, repoName, nil}
	}
	return resultOrError{nil, repoName, err}
}

func checkDiff(regex *regexp.Regexp, from *object.Commit, to *object.Commit,
	foundCommits []*object.Commit) ([]*object.Commit, error) {
	fromTree, err := from.Tree()
	if err != nil {
		return nil, err
	}
	toTree, err := to.Tree()
	if err != nil {
		return nil, err
	}
	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			return nil, err
		}
		if !regex.MatchString(patch.String()) {
			continue
		}
		foundCommits = append(foundCommits, to)

		// returning the first match is enough
		return foundCommits, nil
	}
	return foundCommits, nil
}

func printCommit(commit *object.Commit, repoName string) {

	// This could fail, since there could be duplicates.
	// See https://stackoverflow.com/questions/72864903
	partialHash := commit.Hash.String()[:8]

	fmt.Printf("%s %s %s %s\n",
		partialHash,
		repoName,
		commit.Author.When.Format("2006-01-02T15:04"),
		commit.Author.Name)
}
