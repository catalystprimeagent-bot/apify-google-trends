package main

import "errors"

var (
	errEmptyInput = errors.New("empty input")
	errNoTerms    = errors.New("no valid search terms")
)
