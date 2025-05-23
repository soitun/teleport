/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package utils

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

func TestUserMessageFromError(t *testing.T) {
	// Behavior is different in debug
	defaultLogger := slog.Default()

	var leveler slog.LevelVar
	leveler.Set(slog.LevelInfo)
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		slog.SetDefault(defaultLogger)
	})

	tests := []struct {
		comment   string
		inError   error
		outString string
	}{
		{
			comment:   "outputs x509-specific unknown authority message",
			inError:   trace.Wrap(x509.UnknownAuthorityError{}),
			outString: "WARNING:\n\n  The proxy you are connecting to has presented a",
		},
		{
			comment:   "outputs x509-specific invalid certificate message",
			inError:   trace.Wrap(x509.CertificateInvalidError{}),
			outString: "WARNING:\n\n  The certificate presented by the proxy is invalid",
		},
		{
			comment:   "outputs user message as provided",
			inError:   trace.Errorf("bad thing occurred"),
			outString: "\x1b[31mERROR: \x1b[0mbad thing occurred",
		},
	}

	for _, tt := range tests {
		message := UserMessageFromError(tt.inError)
		require.Contains(t, message, tt.outString)
	}
}

// TestEscapeControl tests escape control
func TestEscapeControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in  string
		out string
	}{
		{
			in:  "hello, world!",
			out: "hello, world!",
		},
		{
			in:  "hello,\nworld!",
			out: `"hello,\nworld!"`,
		},
		{
			in:  "hello,\r\tworld!",
			out: `"hello,\r\tworld!"`,
		},
	}

	for i, tt := range tests {
		require.Equal(t, tt.out, EscapeControl(tt.in), fmt.Sprintf("test case %v", i))
	}
}

// TestAllowWhitespace tests escape control that allows (some) whitespace characters.
func TestAllowWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in  string
		out string
	}{
		{
			in:  "hello, world!",
			out: "hello, world!",
		},
		{
			in:  "hello,\nworld!",
			out: "hello,\nworld!",
		},
		{
			in:  "\thello, world!",
			out: "\thello, world!",
		},
		{
			in:  "\t\thello, world!",
			out: "\t\thello, world!",
		},
		{
			in:  "hello, world!\n",
			out: "hello, world!\n",
		},
		{
			in:  "hello, world!\n\n",
			out: "hello, world!\n\n",
		},
		{
			in:  string([]byte{0x68, 0x00, 0x68}),
			out: "\"h\\x00h\"",
		},
		{
			in:  string([]byte{0x68, 0x08, 0x68}),
			out: "\"h\\bh\"",
		},
		{
			in:  string([]int32{0x00000008, 0x00000009, 0x00000068}),
			out: "\"\\b\"\th",
		},
		{
			in:  string([]int32{0x00000090}),
			out: "\"\\u0090\"",
		},
		{
			in:  "hello,\r\tworld!",
			out: `"hello,\r"` + "\tworld!",
		},
		{
			in:  "hello,\n\r\tworld!",
			out: "hello,\n" + `"\r"` + "\tworld!",
		},
		{
			in:  "hello,\t\n\r\tworld!",
			out: "hello,\t\n" + `"\r"` + "\tworld!",
		},
	}

	for i, tt := range tests {
		require.Equal(t, tt.out, AllowWhitespace(tt.in), fmt.Sprintf("test case %v", i))
	}
}

func TestUpdateAppUsageTemplate(t *testing.T) {
	makeApp := func(usageWriter io.Writer) *kingpin.Application {
		app := InitCLIParser("TestUpdateAppUsageTemplate", "some help message")
		app.UsageWriter(usageWriter)
		app.Terminate(func(int) {})

		app.Command("hello", "Hello.")

		create := app.Command("create", "Create.")
		create.Command("box", "Box.")
		create.Command("rocket", "Rocket.")
		return app
	}

	tests := []struct {
		name           string
		inputArgs      []string
		outputContains string
	}{
		{
			name:      "command width aligned for app help",
			inputArgs: []string{},
			outputContains: `
Commands:
  help          Show help.
  hello         Hello.
  create box    Box.
  create rocket Rocket.
`,
		},
		{
			name:      "command width aligned for command help",
			inputArgs: []string{"create"},
			outputContains: `
Commands:
  create box    Box.
  create rocket Rocket.
`,
		},
		{
			name:      "command width aligned for unknown command error",
			inputArgs: []string{"unknown"},
			outputContains: `
Commands:
  help          Show help.
  hello         Hello.
  create box    Box.
  create rocket Rocket.
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("help flag", func(t *testing.T) {
				var buffer bytes.Buffer
				app := makeApp(&buffer)
				args := append(tt.inputArgs, "--help")
				UpdateAppUsageTemplate(app, args)

				app.Usage(args)
				require.Contains(t, buffer.String(), tt.outputContains)
			})

			t.Run("help command", func(t *testing.T) {
				var buffer bytes.Buffer
				app := makeApp(&buffer)
				args := append([]string{"help"}, tt.inputArgs...)
				UpdateAppUsageTemplate(app, args)

				// HelpCommand is triggered on PreAction during Parse.
				// See kingpin.Application.init for more details.
				_, err := app.Parse(args)
				require.NoError(t, err)
				require.Contains(t, buffer.String(), tt.outputContains)
			})
		})
	}
}
