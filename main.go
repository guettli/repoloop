package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const maxCommits = 100

var minDate = time.Now().AddDate(0, -18, 0)

type resultOrError struct {
	commit   *object.Commit
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
	resultChannel := make(chan resultOrError, 100) // size of channel does not matter
	wg := sync.WaitGroup{}
	for _, f := range files {
		if f.IsDir() {
			path := filepath.Join(dir, f.Name())
			repo, err := git.PlainOpen(path)
			if err != nil {
				fmt.Printf("%s %v\n", f.Name(), err)
				continue
			}
			wg.Add(1)
			go func(regex *regexp.Regexp, repo *git.Repository, resultChannel chan resultOrError, repoName string) {
				defer wg.Done()
				_, err := searchLogInRepo(regex, repo, repoName, resultChannel)
				if err != nil {
					panic(err) // todo: create channel for errors
				}
				fmt.Printf("done with %s\n", repoName)
			}(regex, repo, resultChannel, f.Name())
		}
	}
	return readResults(&wg, resultChannel)
}

func readResults(wg *sync.WaitGroup, resultChannel chan resultOrError) error {
	done := make(chan any)
	go func() {
		wg.Wait()
		close(done)
	}()
	for {
		select {
		case result := <-resultChannel:
			if result.err != nil {
				return result.err
			}
			printCommit(result.commit, result.repoName)
		case <-done:
			// all goroutines are done
			close(resultChannel)
			return nil
		}
	}
}

type stopIterError struct{}

func (e stopIterError) Error() string {
	return "stop"
}

func searchLogInRepo(regex *regexp.Regexp, repo *git.Repository, repoName string,
	resultChannel chan resultOrError) (foundCommits int, err error) {
	options := git.LogOptions{Order: git.LogOrderCommitterTime}
	cIter, err := repo.Log(&options)
	if err != nil {
		return foundCommits, err
	}

	err = cIter.ForEach(func(commit *object.Commit) error {
		switch {
		case len(commit.ParentHashes) == 0:
			return nil
		case len(commit.ParentHashes) == 1:
			parentCommit, err := repo.CommitObject(commit.ParentHashes[0])
			if err != nil {
				return err
			}
			foundCommitsSub, err := checkDiff(regex, parentCommit, commit, repoName, resultChannel)
			if err != nil {
				return err
			}
			foundCommits += foundCommitsSub

			if foundCommits >= maxCommits {
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
		return foundCommits, nil
	}
	return foundCommits, err
}

func checkDiff(regex *regexp.Regexp, from *object.Commit, to *object.Commit,
	repoName string,
	resultChannel chan resultOrError) (int, error) {
	foundCommits := 0
	fromTree, err := from.Tree()
	if err != nil {
		return 0, err
	}
	toTree, err := to.Tree()
	if err != nil {
		return 0, err
	}
	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return 0, err
	}
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			return 0, err
		}

		if !regex.MatchString(patch.String()) {
			continue
		}
		foundCommits++
		resultChannel <- resultOrError{
			commit:   to,
			repoName: repoName,
			err:      nil,
		}
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
