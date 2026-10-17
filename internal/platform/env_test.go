package platform

import (
	"slices"
	"testing"
)

// fakeEnv is an environment of the test's own choosing.
type fakeEnv map[string]string

func (e fakeEnv) Get(name string) (string, bool) {
	value, ok := e[name]
	return value, ok
}

func TestExpandReplacesWhatTheEnvironmentHolds(t *testing.T) {
	env := fakeEnv{
		"TEMP":              `C:\Temp`,
		"LOCALAPPDATA":      `C:\Users\probe\AppData\Local`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "one variable", path: `%TEMP%\sweep`, want: `C:\Temp\sweep`},
		{
			name: "two variables",
			path: `%LOCALAPPDATA%\sweep\%TEMP%`,
			want: `C:\Users\probe\AppData\Local\sweep\C:\Temp`,
		},
		{name: "a name with parentheses", path: `%ProgramFiles(x86)%\sweep`, want: `C:\Program Files (x86)\sweep`},
		{name: "no variable", path: `C:\Windows\Temp`, want: `C:\Windows\Temp`},
		{
			name: "a variable that is not set stays as written",
			path: `%SWEEP_NO_SUCH_VAR%\cache`,
			want: `%SWEEP_NO_SUCH_VAR%\cache`,
		},
		{name: "a lone percent", path: `100%\cache`, want: `100%\cache`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Expand(test.path, env); got != test.want {
				t.Errorf("Expand(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestEnvNamesListsTheVariablesOfAPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "in order, with a repeat",
			path: `%TEMP%\%LOCALAPPDATA%\%TEMP%`,
			want: []string{"TEMP", "LOCALAPPDATA", "TEMP"},
		},
		{name: "a name with parentheses", path: `%ProgramFiles(x86)%\x`, want: []string{"ProgramFiles(x86)"}},
		{name: "none", path: `C:\Windows\Temp`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := EnvNames(test.path); !slices.Equal(got, test.want) {
				t.Errorf("EnvNames(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func TestOSEnvReadsTheProcessEnvironment(t *testing.T) {
	t.Setenv("SWEEP_TEST_VAR", "yes")

	if value, ok := (OSEnv{}).Get("SWEEP_TEST_VAR"); !ok || value != "yes" {
		t.Errorf("Get(SWEEP_TEST_VAR) = %q, %v, want yes, true", value, ok)
	}
	if _, ok := (OSEnv{}).Get("SWEEP_NO_SUCH_VAR"); ok {
		t.Error("Get of a variable that is not set reported it as set")
	}
}
