package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

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
	for _, f := range files {
		if f.IsDir() {
			path := filepath.Join(dir, f.Name())
			r, err := git.PlainOpen(path)
			if err != nil {
				fmt.Printf("%s %v", f.Name(), err)
				continue
			}
			foundCommits, err := searchLogInRepo(regex, f.Name(), r)
			if err != nil {
				return err
			}
			for _, commit := range foundCommits {
				printCommit(commit, f.Name())
			}
		}
	}
	return nil
}

func searchLogInRepo(regex *regexp.Regexp, repoName string, r *git.Repository) (foundCommits []*object.Commit, err error) {
	fmt.Printf("---------------- repo %v\n", repoName)
	options := git.LogOptions{Order: git.LogOrderCommitterTime}
	cIter, err := r.Log(&options)
	if err != nil {
		return nil, err
	}
	err = cIter.ForEach(func(c *object.Commit) error {
		switch {
		case len(c.ParentHashes) == 0:
			return nil
		case len(c.ParentHashes) == 1:
			parentCommit, err := r.CommitObject(c.ParentHashes[0])
			if err != nil {
				return err
			}
			foundCommits, err = checkDiff(regex, repoName, parentCommit, c, foundCommits)
			if err != nil {
				return err
			}

		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return foundCommits, nil
}

func checkDiff(regex *regexp.Regexp, repoName string, from *object.Commit, to *object.Commit,
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
