package main

import "fmt"

// Models is the round-robin pool of gpt-* models to test.
var Models = []string{
	"gpt-5.2",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.5",
	"gpt-5.3-codex",
}

// ModelForRequest returns the model for the given request sequence number.
func ModelForRequest(seq int64) string {
	return Models[seq%int64(len(Models))]
}

// Tasks is the pool of task variants. BuildPrompt picks one and wraps it.
var Tasks = []string{
	"reverses a string",
	"checks if a number is prime",
	"parses an ISO 8601 date string",
	"merges two sorted lists into one sorted list",
	"counts word frequency in a text",
	"flattens a nested list of integers",
	"computes the nth Fibonacci number iteratively",
	"converts a hex color string to an RGB tuple",
	"validates an email address using a regex",
	"removes duplicates from a list while preserving order",
	"computes the longest common prefix of a list of strings",
	"capitalizes the first letter of each word in a sentence",
	"finds the second largest unique value in a list of integers",
	"transposes a 2D matrix represented as a list of lists",
	"converts a Roman numeral string to an integer",
	"computes the GCD of two positive integers",
	"checks if two strings are anagrams of each other",
	"flattens a deeply nested dictionary using dot notation",
	"computes the moving average of a list with given window size",
	"groups items in a list by a key function",
}

// BuildPrompt wraps a task into the canonical user message.
func BuildPrompt(task string) string {
	return fmt.Sprintf("Write a Python function that %s. Include a brief docstring.", task)
}
