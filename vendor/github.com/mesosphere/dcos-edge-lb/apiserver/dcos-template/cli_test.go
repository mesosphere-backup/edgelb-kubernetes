package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos-template/config"
)

func TestCLI_ParseFlags(t *testing.T) {
	t.Parallel()

	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	cases := []struct {
		name string
		f    []string
		e    *config.Config
		err  bool
	}{
		{
			"config",
			[]string{"-config", f.Name()},
			&config.Config{},
			false,
		},
		{
			"config_bad_path",
			[]string{"-config", "/not/a/real/path"},
			nil,
			true,
		},
		{
			"exec",
			[]string{"-exec", "command"},
			&config.Config{
				Exec: &config.ExecConfig{
					Enabled: config.Bool(true),
					Command: config.String("command"),
				},
			},
			false,
		},
		{
			"exec-kill-signal",
			[]string{"-exec-kill-signal", "SIGUSR1"},
			&config.Config{
				Exec: &config.ExecConfig{
					KillSignal: config.Signal(syscall.SIGUSR1),
				},
			},
			false,
		},
		{
			"exec-kill-timeout",
			[]string{"-exec-kill-timeout", "10s"},
			&config.Config{
				Exec: &config.ExecConfig{
					KillTimeout: config.TimeDuration(10 * time.Second),
				},
			},
			false,
		},
		{
			"exec-reload-signal",
			[]string{"-exec-reload-signal", "SIGUSR1"},
			&config.Config{
				Exec: &config.ExecConfig{
					ReloadSignal: config.Signal(syscall.SIGUSR1),
				},
			},
			false,
		},
		{
			"exec-splay",
			[]string{"-exec-splay", "10s"},
			&config.Config{
				Exec: &config.ExecConfig{
					Splay: config.TimeDuration(10 * time.Second),
				},
			},
			false,
		},
		{
			"kill-signal",
			[]string{"-kill-signal", "SIGUSR1"},
			&config.Config{
				KillSignal: config.Signal(syscall.SIGUSR1),
			},
			false,
		},
		{
			"log-level",
			[]string{"-log-level", "DEBUG"},
			&config.Config{
				LogLevel: config.String("DEBUG"),
			},
			false,
		},
		{
			"max-stale",
			[]string{"-max-stale", "10s"},
			&config.Config{
				MaxStale: config.TimeDuration(10 * time.Second),
			},
			false,
		},
		{
			"pid-file",
			[]string{"-pid-file", "/var/pid/file"},
			&config.Config{
				PidFile: config.String("/var/pid/file"),
			},
			false,
		},
		{
			"reload-signal",
			[]string{"-reload-signal", "SIGUSR1"},
			&config.Config{
				ReloadSignal: config.Signal(syscall.SIGUSR1),
			},
			false,
		},
		{
			"retry",
			[]string{"-retry", "30s"},
			&config.Config{
				Retry: config.TimeDuration(30 * time.Second),
			},
			false,
		},
		{
			"syslog",
			[]string{"-syslog"},
			&config.Config{
				Syslog: &config.SyslogConfig{
					Enabled: config.Bool(true),
				},
			},
			false,
		},
		{
			"syslog-facility",
			[]string{"-syslog-facility", "LOCAL0"},
			&config.Config{
				Syslog: &config.SyslogConfig{
					Facility: config.String("LOCAL0"),
				},
			},
			false,
		},
		{
			"template",
			[]string{"-template", "/tmp/in.tpl"},
			&config.Config{
				Templates: &config.TemplateConfigs{
					&config.TemplateConfig{
						Source: config.String("/tmp/in.tpl"),
					},
				},
			},
			false,
		},
		{
			"wait_min",
			[]string{"-wait", "10s"},
			&config.Config{
				Wait: &config.WaitConfig{
					Min: config.TimeDuration(10 * time.Second),
					Max: config.TimeDuration(40 * time.Second),
				},
			},
			false,
		},
		{
			"wait_min_max",
			[]string{"-wait", "10s:30s"},
			&config.Config{
				Wait: &config.WaitConfig{
					Min: config.TimeDuration(10 * time.Second),
					Max: config.TimeDuration(30 * time.Second),
				},
			},
			false,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d_%s", i, tc.name), func(t *testing.T) {
			var out bytes.Buffer
			cli := NewCLI(&out, &out)

			a, _, _, _, err := cli.ParseFlags(tc.f)
			if (err != nil) != tc.err {
				t.Fatal(err)
			}

			if tc.e != nil {
				tc.e = config.DefaultConfig().Merge(tc.e)
				tc.e.Finalize()
			}
			if !reflect.DeepEqual(tc.e, a) {
				t.Errorf("\nexp: %#v\nact: %#v\nout: %q", tc.e, a, out.String())
			}
		})
	}
}

func TestCLI_Run(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		f    func(*testing.T, int, string)
	}{
		{
			"help",
			[]string{"-h"},
			func(t *testing.T, i int, s string) {
				if i != 0 {
					t.Error("expected 0 exit")
				}
			},
		},
		{
			"version",
			[]string{"-v"},
			func(t *testing.T, i int, s string) {
				if i != 0 {
					t.Error("expected 0 exit")
				}
			},
		},
		{
			"too_many_args",
			[]string{"foo", "bar", "baz"},
			func(t *testing.T, i int, s string) {
				if i == 0 {
					t.Error("expected error")
				}
				if !strings.Contains(s, "extra args") {
					t.Errorf("\nexp: %q\nact: %q", "extra args", s)
				}
			},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d_%s", i, tc.name), func(t *testing.T) {
			var out bytes.Buffer
			cli := NewCLI(&out, &out)

			tc.args = append([]string{"consul-template"}, tc.args...)
			exit := cli.Run(tc.args)
			tc.f(t, exit, out.String())
		})
	}
}
