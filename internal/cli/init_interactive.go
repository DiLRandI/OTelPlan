package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/suggest"
)

var errInteractiveInputEnded = errors.New("interactive input ended before all suggestions were reviewed")

func reviewStarterCandidates(ctx context.Context, input io.Reader, prompts io.Writer,
	candidates []suggest.Suggestion,
) ([]suggest.Suggestion, error) {
	reader := bufio.NewReader(input)
	selected := make([]suggest.Suggestion, 0, len(candidates))

	for _, candidate := range candidates {
		accepted, err := askForStarterCandidate(ctx, reader, prompts, candidate)
		if err != nil {
			return nil, err
		}

		if accepted {
			selected = append(selected, candidate)
		}
	}

	return selected, nil
}

func askForStarterCandidate(ctx context.Context, reader *bufio.Reader, prompts io.Writer,
	candidate suggest.Suggestion,
) (bool, error) {
	_, err := fmt.Fprintf(prompts, "SUGGESTION %s confidence=%s score=%d\n  evidence: %s\n",
		candidate.SymbolID, candidate.Confidence, candidate.Score, strings.Join(candidate.Evidence, "; "))
	if err != nil {
		return false, fmt.Errorf("write interactive suggestion: %w", err)
	}

	for {
		_, err = fmt.Fprint(prompts, "Include this target? [y/N] ")
		if err != nil {
			return false, fmt.Errorf("write interactive prompt: %w", err)
		}

		answer, readErr := readStarterAnswer(ctx, reader)
		if readErr != nil {
			return false, readErr
		}

		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		default:
			_, err = fmt.Fprintln(prompts, "Enter y or n.")
			if err != nil {
				return false, fmt.Errorf("write interactive guidance: %w", err)
			}
		}
	}
}

func readStarterAnswer(ctx context.Context, reader *bufio.Reader) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("interactive review canceled: %w", err)
	}

	type result struct {
		line string
		err  error
	}

	response := make(chan result, 1)

	go func() {
		line, err := reader.ReadString('\n')
		response <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("interactive review canceled: %w", ctx.Err())
	case answer := <-response:
		err = ctx.Err()
		if err != nil {
			return "", fmt.Errorf("interactive review canceled: %w", err)
		}

		if errors.Is(answer.err, io.EOF) && answer.line == "" {
			return "", errInteractiveInputEnded
		}

		if answer.err != nil && !errors.Is(answer.err, io.EOF) {
			return "", fmt.Errorf("read interactive answer: %w", answer.err)
		}

		return answer.line, nil
	}
}
