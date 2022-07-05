package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const maxCommits = 100

var minDate = time.Now().AddDate(0, -18, 0)

type resultOrError struct {
	commits []*object.Commit
	err     error
}

type channelOfRepo struct {
	channel  chan resultOrError
	repoName string
}

func startProfiling() {
	f, err := os.Create("cpu.pprof")
	if err != nil {
		panic(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM) // subscribe to system signals
	onKill := func(c chan os.Signal) {
		select {
		case <-c:
			defer f.Close()
			defer pprof.StopCPUProfile()
			defer os.Exit(0)
		}
	}
	// try to handle os interrupt(signal terminated)
	go onKill(c)
}

func main() {
	startProfiling()
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
	var resultChannels []channelOfRepo
	for _, f := range files {
		if f.IsDir() {
			path := filepath.Join(dir, f.Name())
			repo, err := git.PlainOpen(path)
			if err != nil {
				fmt.Printf("%s %v", f.Name(), err)
				continue
			}
			resultChannel := make(chan resultOrError, 1)
			resultChannels = append(resultChannels, channelOfRepo{resultChannel, f.Name()})
			go searchLogInRepo(regex, repo, resultChannel)
		}
	}
	fmt.Println("All goroutines got started")
	for _, channelOfRepo := range resultChannels {
		fmt.Printf("Waiting for %s\n", channelOfRepo.repoName)
		result, ok := <-channelOfRepo.channel
		if !ok {
			return fmt.Errorf("reading from closed channel")
		}
		if result.err != nil {
			return result.err
		}
		for _, commit := range result.commits {
			printCommit(commit, channelOfRepo.repoName)
		}
	}
	return nil
}

type stopIterError struct{}

func (e stopIterError) Error() string {
	return "stop"
}

func searchLogInRepo(regex *regexp.Regexp, repo *git.Repository, resultChannel chan resultOrError) {
	options := git.LogOptions{Order: git.LogOrderCommitterTime}
	cIter, err := repo.Log(&options)
	if err != nil {
		resultChannel <- resultOrError{nil, err}
		return
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
		resultChannel <- resultOrError{foundCommits, nil}
		return
	}
	resultChannel <- resultOrError{nil, err}
	return
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
