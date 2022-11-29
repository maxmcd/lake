package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type Config struct {
	Env         []string
	Interpreter []string
	File        string
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var config Config
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		panic(err)
	}
	cmd := exec.Command("")
	cmd.Path = config.Interpreter[0]
	cmd.Args = append(config.Interpreter, config.File)
	cmd.Args = append(cmd.Args, os.Args[1:]...)
	fmt.Println(cmd.Args)
	cmd.Env = os.Environ()
	for _, val := range config.Env {
		cmd.Env = append(cmd.Env, val)
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(cmd.ProcessState.ExitCode())
}
