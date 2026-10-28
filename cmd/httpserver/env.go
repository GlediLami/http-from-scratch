package main

import "os"

type Env struct {
	Port string
}

// NewEnv acts as a constructor function
func NewEnv() *Env {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	return &Env{
		Port: port,
	}
}
