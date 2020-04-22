package gqltesting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/errors"
)

// Test is a GraphQL test case to be used with RunTest(s).
type Test struct {
	Context        context.Context
	Schema         *graphql.Schema
	Query          string
	OperationName  string
	Variables      map[string]interface{}
	ExpectedResult string
	ExpectedErrors []*errors.QueryError
}

// RunTests runs the given GraphQL test cases as subtests.
func RunTests(t *testing.T, tests []*Test) {
	if len(tests) == 1 {
		RunTest(t, tests[0])
		return
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			RunTest(t, test)
		})
	}
}

var diffAvailableOnSystem bool
var checkDiffOnce sync.Once

// RunTest runs a single GraphQL test case.
func RunTest(t *testing.T, test *Test) {
	if test.Context == nil {
		test.Context = context.Background()
	}
	result := test.Schema.Exec(test.Context, test.Query, test.OperationName, test.Variables)

	checkErrors(t, test.ExpectedErrors, result.Errors)

	if test.ExpectedResult == "" {
		if result.Data != nil {
			t.Fatalf("got: %s", result.Data)
			t.Fatalf("want: null")
		}
		return
	}

	// Verify JSON to avoid red herring errors.
	got, err := formatJSON(result.Data)
	if err != nil {
		t.Fatalf("got: invalid JSON: %s", err)
	}
	want, err := formatJSON([]byte(test.ExpectedResult))
	if err != nil {
		t.Fatalf("want: invalid JSON: %s", err)
	}

	if !bytes.Equal(got, want) {
		// ONCE, check to see if diff is on this system.
		checkDiffOnce.Do(func() {
			_, err := exec.LookPath("diff")
			if err == nil {
				diffAvailableOnSystem = true
			}
		})

		if !diffAvailableOnSystem {
			t.Logf("got:  %s", got)
			t.Logf("want: %s", want)
		} else {
			// Run diff on the output so that it's possible to tell what changed.
			diff, err := diffJSON(want, got)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("Did not get what we want:\n%s", diff)
		}

		t.Fail()
	}
}

func diffJSON(expected []byte, received []byte) (string, error) {

	// write two files and call diff on them
	tmpDir, err := ioutil.TempDir("", "graphql-go-diff")
	if err != nil {
		return "", err
	}

	expectedPath := path.Join(tmpDir, "expected.json")
	receivedPath := path.Join(tmpDir, "received.json")

	err = ioutil.WriteFile(expectedPath, expected, 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(receivedPath, received, 0644)
	if err != nil {
		return "", err
	}

	diffCmd := exec.Command("diff", "-u", "-Lexpected.json", "-Lactual.json", expectedPath, receivedPath)
	diffOutput, err := diffCmd.Output()

	if err == nil {
		return "", fmt.Errorf("Unexpected error: We should only be calling diff on output that is not what was expected")
	}

	if err.Error() != "exit status 1" {
		return "", fmt.Errorf("Unexpected error runing diff: %w", err)
	}

	return string(diffOutput), nil
}

func formatJSON(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	formatted, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func checkErrors(t *testing.T, want, got []*errors.QueryError) {
	sortErrors(want)
	sortErrors(got)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected error: got %+v, want %+v", got, want)
	}
}

func sortErrors(errors []*errors.QueryError) {
	if len(errors) <= 1 {
		return
	}
	sort.Slice(errors, func(i, j int) bool {
		return fmt.Sprintf("%s", errors[i].Path) < fmt.Sprintf("%s", errors[j].Path)
	})
}
