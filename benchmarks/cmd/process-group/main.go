package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const terminationGrace = 5 * time.Second

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(arguments []string, stderr io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprintln(stderr, "usage: process-group <command> [arguments...]")
		return 2
	}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "start process group: %v\n", err)
		return 1
	}

	waited := make(chan error, 1)
	go func() {
		waited <- command.Wait()
	}()

	select {
	case err := <-waited:
		return processExitCode(err)
	case received := <-signals:
		signalValue, ok := received.(syscall.Signal)
		if !ok {
			signalValue = syscall.SIGTERM
		}
		if err := command.Process.Signal(signalValue); err != nil {
			fmt.Fprintf(stderr, "terminate measured process: %v\n", err)
		}
		timer := time.NewTimer(terminationGrace)
		defer timer.Stop()
		select {
		case err := <-waited:
			return processExitCode(err)
		case <-timer.C:
			if err := command.Process.Kill(); err != nil {
				fmt.Fprintf(stderr, "kill measured process: %v\n", err)
			}
			return processExitCode(<-waited)
		}
	}
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return 1
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	if !ok {
		return 1
	}
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}
