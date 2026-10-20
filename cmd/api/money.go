package main

import (
	"errors"
	"strconv"
	"strings"
)

func parseAmountCents(amount string) (int64, error) {
	parte := strings.Split(amount, ".")
	if len(parte) != 2 {
		return 0, errors.New("invalid_amount")
	}
	if parte[0] == "" {
		return 0, errors.New("invalid_amount")
	}
	if len(parte[1]) != 2 {
		return 0, errors.New("invalid_amount")
	}

	total := parte[0] + parte[1]
	for _, t := range total {
		if t < '0' || t > '9' {
			return 0, errors.New("invalid_amount")
		}
	}
	centavos, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, errors.New("invalid_amount")
	}
	return centavos, nil
}
