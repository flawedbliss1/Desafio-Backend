package main

import (
	"fmt"
	"testing"
)

func TestAmountCents(t *testing.T) {
	cents, err := parseAmountCents("0.05")
	if err != nil {
		t.Fatal(err)
	}
	if cents != 5 {
		t.Fatalf("esperava 5, recebi %d", cents)
	}
	fmt.Print(cents)
}

func TestAmountCentsInvalid(t *testing.T) {

	entradas := []string{
		"0.052",
		"-1.00",
		"a.00",
		"92233720368547758.08",
	}
	for _, entrada := range entradas {
		cents, err := parseAmountCents(entrada)
		if err == nil {
			t.Fatalf("esperava erro para %q; recebi %d centavos", entrada, cents)
		}

	}
}
