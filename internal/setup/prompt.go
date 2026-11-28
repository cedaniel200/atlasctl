package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Prompter asks one question. The question is already rendered from the
// catalog; the prompter only handles terminal input and output.
type Prompter interface {
	Ask(question, defaultValue string) (string, error)
}

// Terminal is the injected terminal reader used by the interview.
type Terminal struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewTerminal builds a prompter over the given streams.
func NewTerminal(reader io.Reader, writer io.Writer) *Terminal {
	if reader == nil {
		reader = strings.NewReader("")
	}
	if writer == nil {
		writer = io.Discard
	}
	return &Terminal{reader: bufio.NewReader(reader), writer: writer}
}

// Ask prints the question and returns the answer or the default on an empty
// line or EOF.
func (t *Terminal) Ask(question, defaultValue string) (string, error) {
	if _, err := fmt.Fprint(t.writer, question); err != nil {
		return "", err
	}
	line, err := t.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		if errors.Is(err, io.EOF) {
			return defaultValue, nil
		}
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func ask(request Request, id string, args map[string]string, defaultValue string) (string, error) {
	if request.Prompter == nil {
		return defaultValue, nil
	}
	if request.Catalog == nil {
		return "", errors.New("setup: a prompt catalog is required")
	}
	answer, err := request.Prompter.Ask(request.Catalog.T(id, args), defaultValue)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultValue, nil
	}
	return answer, nil
}
