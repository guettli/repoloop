# repoloop

Loop over N git repos. At the moment this is my playground for learning go.

Imagine you have one directory containing many git repos.

You want to loop over all git repos and execute `git log -G ...` or similar
commands in each git repo.

This code uses [go-git](https://github.com/go-git/go-git) which is a pure Golang
implementation of git.

# Development Install

```
> git clone git@github.com:guettli/repoloop.git

> cd repoloop

> go mod tidy

> go run main.go search-term path_to_directory_containing_git_repos

```

