package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	h, err := bcrypt.GenerateFromPassword([]byte("Screen2Code-Test-2026!"), bcrypt.DefaultCost)
	if err != nil { panic(err) }
	fmt.Print(string(h))
}
