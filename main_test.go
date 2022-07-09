package main

import (
	"regexp"
	"testing"
)

func BenchmarkSearchLog(b *testing.B) {
	regex, _ := regexp.Compile("fooooooooooooo")
	for i := 0; i < b.N; i++ {
		SearchLog(regex, "tmp")
	}
}
