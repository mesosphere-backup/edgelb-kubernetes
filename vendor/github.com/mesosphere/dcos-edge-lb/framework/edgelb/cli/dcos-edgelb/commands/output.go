package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func printStdout(s string) error {
	_, err := fmt.Fprint(os.Stdout, s)
	return err
}

func printStderr(s string) error {
	_, err := fmt.Fprint(os.Stderr, s)
	return err
}

func printStdoutLn(s string) error {
	return printStdout(fmt.Sprintln(s))
}

func printStderrLn(s string) error {
	return printStderr(fmt.Sprintln(s))
}

func printNonemptyLines(s string) error {
	outputStr := ""
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		s := scanner.Text()
		if len(strings.Fields(s)) == 0 {
			continue
		}
		outputStr += fmt.Sprintln(s)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error parsing string: %s", err)
	}
	return printStdoutLn(outputStr)
}
