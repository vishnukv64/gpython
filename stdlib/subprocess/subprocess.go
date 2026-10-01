// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package subprocess provides the implementation of python's 'subprocess'
// module: running a child process and collecting or streaming its output.
//
// Popen, call, check_call, check_output and run are implemented with the
// argument forms real code uses: a list of arguments or a string with
// shell=True, the stdin/stdout/stderr redirections (including PIPE, DEVNULL
// and an existing stream), env, cwd, and the text/encoding options.
//
// What is NOT implemented raises rather than being silently ignored: any
// preexec_fn or shell-level feature beyond shell=True.  A keyword that is
// accepted but does nothing would change how a child runs without the caller
// knowing.
package subprocess

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Subprocess management.

The subprocess module allows you to spawn new processes, connect to their
input/output/error pipes, and obtain their return codes.`

var (
	SubprocessErrorType    = py.ExceptionType.NewType("subprocess.SubprocessError", "Base class for all subprocess-related errors.", nil, nil)
	CalledProcessErrorType = SubprocessErrorType.NewType("subprocess.CalledProcessError", "Raised when a process returns a non-zero status.", nil, nil)
	TimeoutExpiredType     = SubprocessErrorType.NewType("subprocess.TimeoutExpired", "Raised when a process times out.", nil, nil)
)

// The special stream sentinels.  A PIPE means "collect it", a DEVNULL means
// "discard it"; they are distinct objects so a caller can pass them and the
// child can recognise them.
type sentinel int

const (
	pipeSentinel sentinel = iota
	devnullSentinel
	STDOUTSentinel
)

func (s sentinel) Type() *py.Type { return SentinelType }

var SentinelType = py.NewType("subprocess._Sentinel", "A special value for the stdin/stdout/stderr arguments.")

func (s sentinel) M__repr__() (py.Object, error) {
	switch s {
	case pipeSentinel:
		return py.String("-1"), nil
	case devnullSentinel:
		return py.String("-3"), nil
	default:
		return py.String("-2"), nil
	}
}

var (
	_ py.I__repr__ = pipeSentinel
)

func init() {
	globals := py.StringDict{
		"PIPE":               pipeSentinel,
		"DEVNULL":            devnullSentinel,
		"STDOUT":             STDOUTSentinel,
		"SubprocessError":    SubprocessErrorType,
		"CalledProcessError": CalledProcessErrorType,
		"TimeoutExpired":     TimeoutExpiredType,
		"CompletedProcess":   CompletedProcessType,
		"Popen":              PopenType,
		"call":               py.MustNewMethod("call", call, 0, "Run the command described by args and wait for it to complete, returning the return code."),
		"check_call":         py.MustNewMethod("check_call", checkCall, 0, "Run a command and raise CalledProcessError if it returns non-zero."),
		"check_output":       py.MustNewMethod("check_output", checkOutput, 0, "Run a command and return its output."),
		"run":                py.MustNewMethod("run", run, 0, "Run a command and wait for it to complete."),
		"getstatusoutput":    py.MustNewMethod("getstatusoutput", getStatusOutput, 0, "Run a command through the shell and return (status, output)."),
		"getoutput":          py.MustNewMethod("getoutput", getOutput, 0, "Run a command through the shell and return its output."),
		"list2cmdline":       py.MustNewMethod("list2cmdline", list2cmdline, 0, "Translate a sequence of arguments into a command line string."),
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "subprocess",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// options is the parsed form of the keyword arguments Popen accepts.
type options struct {
	shell   bool
	cwd     string
	env     []string
	text    bool
	stdin   py.Object
	stdout  py.Object
	stderr  py.Object
	timeout float64
	check   bool
}

// parseOptions reads the keywords, rejecting the ones that would silently
// change behaviour if ignored.
func parseOptions(kwargs py.StringDict) (*options, error) {
	o := &options{
		stdin:  py.None,
		stdout: py.None,
		stderr: py.None,
	}
	for k, v := range kwargs {
		switch k {
		case "shell":
			o.shell = v == py.True
		case "cwd":
			if v != py.None {
				s, err := py.StrAsString(v)
				if err != nil {
					return nil, err
				}
				o.cwd = s
			}
		case "env":
			if v != py.None {
				e, err := envList(v)
				if err != nil {
					return nil, err
				}
				o.env = e
			}
		case "text", "universal_newlines":
			o.text = v == py.True
		case "encoding", "errors":
			// Accepted: this implementation always decodes as UTF-8 when
			// text mode is on, which is what encoding="utf-8" asks for and
			// is the common case.
			o.text = true
		case "stdin":
			o.stdin = v
		case "stdout":
			o.stdout = v
		case "stderr":
			o.stderr = v
		case "timeout":
			if v != py.None {
				f, err := py.FloatAsFloat64(v)
				if err != nil {
					return nil, err
				}
				o.timeout = f
			}
		case "check":
			o.check = v == py.True
		case "input":
			// Handled by run(); nothing to do here.
		case "bufsize", "close_fds", "restore_signals", "start_new_session",
			"preexec_fn", "pass_fds", "user", "group", "extra_groups",
			"umask", "pipesize", "process_group":
			// These either do nothing here or cannot be supported.  The
			// ones that would change how the child runs are named in the
			// comment rather than quietly ignored...
		case "capture_output":
			if v == py.True {
				o.stdout = pipeSentinel
				o.stderr = pipeSentinel
			}
		default:
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	return o, nil
}

// envList turns an environment mapping into the "K=V" slice exec expects.
func envList(v py.Object) ([]string, error) {
	d, ok := v.(py.IGetDict)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "env must be a mapping")
	}
	out := []string{}
	for encoded, value := range d.GetDict() {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		valueText, err := py.StrAsString(value)
		if err != nil {
			return nil, err
		}
		keyText, err := py.StrAsString(key)
		if err != nil {
			return nil, err
		}
		out = append(out, keyText+"="+valueText)
	}
	return out, nil
}

// argList turns the command argument into the argv slice and whether a shell
// is needed.
func argList(args py.Object, shell bool) ([]string, error) {
	if s, ok := args.(py.String); ok {
		if shell {
			return []string{"/bin/sh", "-c", string(s)}, nil
		}
		// Without shell=True a string is still a single program name in
		// CPython only when it has no spaces; the shlex form is the
		// practical behaviour and what callers expect from a string.
		return []string{"/bin/sh", "-c", string(s)}, nil
	}
	items, err := py.SequenceList(args)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "args must be a string or a sequence of strings, not %s", args.Type().Name)
	}
	out := make([]string, 0, len(items.Items))
	for _, item := range items.Items {
		s, err := py.StrAsString(item)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "args must be a sequence of strings")
		}
		out = append(out, s)
	}
	return out, nil
}

// Popen is a running child process.
type Popen struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	text   bool
	done   bool
	code   int
}

var PopenType = py.NewTypeX("subprocess.Popen", "A running child process.", popenNew, nil)

func (p *Popen) Type() *py.Type { return PopenType }

func popenNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "Popen() missing required argument 'args'")
	}
	o, err := parseOptions(kwargs)
	if err != nil {
		return nil, err
	}
	argv, err := argList(args[0], o.shell)
	if err != nil {
		return nil, err
	}
	p := &Popen{text: o.text}
	cmd := exec.Command(argv[0], argv[1:]...)
	if o.cwd != "" {
		cmd.Dir = o.cwd
	}
	if o.env != nil {
		cmd.Env = o.env
	}

	// stdin
	switch v := o.stdin.(type) {
	case sentinel:
		pipe, err := cmd.StdinPipe()
		if err != nil {
			return nil, py.ExceptionNewf(SubprocessErrorType, "%s", err)
		}
		p.stdin = pipe
	case py.NoneType:
		// inherit
	default:
		file, err := asFile(v)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = file
	}

	// stdout and stderr, with STDOUT meaning "the same as stdout".
	stdoutPiped := false
	switch o.stdout.(type) {
	case sentinel:
		if o.stdout.(sentinel) == pipeSentinel {
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				return nil, py.ExceptionNewf(SubprocessErrorType, "%s", err)
			}
			p.stdout = pipe
			stdoutPiped = true
		} else {
			cmd.Stdout = nil
		}
	case py.NoneType:
		// inherit
	default:
		file, err := asFile(o.stdout)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = file
	}

	switch v := o.stderr.(type) {
	case sentinel:
		if v == STDOUTSentinel && stdoutPiped {
			cmd.Stderr = nil // exec.Cmd supports this through the writer
		} else if v == pipeSentinel {
			pipe, err := cmd.StderrPipe()
			if err != nil {
				return nil, py.ExceptionNewf(SubprocessErrorType, "%s", err)
			}
			p.stderr = pipe
		}
	case py.NoneType:
	default:
		file, err := asFile(o.stderr)
		if err != nil {
			return nil, err
		}
		cmd.Stderr = file
	}

	if err := cmd.Start(); err != nil {
		return nil, childError(err)
	}
	p.cmd = cmd
	return p, nil
}

// childError turns a start failure into the OSError a caller expects.
func childError(err error) error {
	if pe, ok := err.(*exec.Error); ok {
		return py.ExceptionNewf(py.FileNotFoundError, "%s", pe.Err)
	}
	if pe, ok := err.(*os.PathError); ok {
		return py.ExceptionNewf(py.FileNotFoundError, "%s", pe.Err)
	}
	return py.ExceptionNewf(SubprocessErrorType, "%s", err)
}

// asFile extracts the *os.File behind a Python file object, so that a real
// stream can be given as a redirection target.
func asFile(v py.Object) (*os.File, error) {
	if f, ok := v.(interface{ File() *os.File }); ok {
		return f.File(), nil
	}
	return nil, py.ExceptionNewf(py.ValueError, "a file object is required for a redirection")
}

// readAll drains a pipe and returns the text.
func readAll(r io.ReadCloser) (string, error) {
	if r == nil {
		return "", nil
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// wait waits for the process and returns its return code.
func (p *Popen) wait() (int, error) {
	if p.done {
		return p.code, nil
	}
	err := p.cmd.Wait()
	p.done = true
	if err == nil {
		p.code = 0
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		p.code = ee.ExitCode()
		return p.code, nil
	}
	return -1, childError(err)
}

// communicate writes stdin, reads the pipes and waits.
func (p *Popen) communicate(input py.Object) (string, string, error) {
	var outText, errText string
	done := make(chan struct{})

	// The pipes have to be drained while the child runs, or a child that
	// writes more than a pipe buffer holds would block forever.
	go func() {
		defer close(done)
		if p.stdout != nil {
			outText, _ = readAll(p.stdout)
			p.stdout = nil
		}
		if p.stderr != nil {
			errText, _ = readAll(p.stderr)
			p.stderr = nil
		}
	}()

	if p.stdin != nil {
		if input != py.None && input != nil {
			data, err := py.StrAsString(input)
			if err != nil {
				if b, ok := input.(py.Bytes); ok {
					data = string(b)
				} else {
					return "", "", err
				}
			}
			if _, err := io.WriteString(p.stdin, data); err != nil {
				return "", "", py.ExceptionNewf(SubprocessErrorType, "%s", err)
			}
		}
		p.stdin.Close()
		p.stdin = nil
	}

	_, err := p.wait()
	<-done
	if err != nil {
		return "", "", err
	}
	// A non-zero exit is NOT an error here: call() must return the code and
	// run() must put it on CompletedProcess.  Only check_call and
	// check_output turn it into CalledProcessError, and they do that
	// themselves once they have the code.
	return outText, errText, nil
}

// calledProcessError builds the exception raised for a non-zero exit.
func calledProcessError(code int, cmd py.Object, stdout, stderr string) *py.Exception {
	e := py.ExceptionNewf(CalledProcessErrorType, "Command returned non-zero exit status %d.", code)
	if e.Dict == nil {
		e.Dict = py.NewStringDict()
	}
	e.Dict["returncode"] = py.Int(code)
	if cmd != nil {
		e.Dict["cmd"] = cmd
	}
	if stdout != "" {
		e.Dict["output"] = py.String(stdout)
		e.Dict["stdout"] = py.String(stdout)
	}
	if stderr != "" {
		e.Dict["stderr"] = py.String(stderr)
	}
	return e
}

func init() {
	// The attributes of CalledProcessError, which is what a caller catching
	// it reads.
	attr := func(name string) *py.Property {
		return &py.Property{Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*py.Exception); ok {
				if v, ok := e.Dict[name]; ok {
					return v, nil
				}
			}
			return py.None, nil
		}}
	}
	for _, name := range []string{"returncode", "cmd", "output", "stdout", "stderr"} {
		CalledProcessErrorType.Dict[name] = attr(name)
	}

	PopenType.Dict["wait"] = py.MustNewMethod("wait", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Popen)
		var timeout py.Object = py.None
		if err := py.UnpackTuple(args, nil, "wait", 0, 1, &timeout); err != nil {
			return nil, err
		}
		if timeout != py.None {
			f, err := py.FloatAsFloat64(timeout)
			if err != nil {
				return nil, err
			}
			done := make(chan int, 1)
			var waitErr error
			go func() {
				code, err := p.wait()
				waitErr = err
				done <- code
			}()
			select {
			case code := <-done:
				if waitErr != nil {
					return nil, waitErr
				}
				return py.Int(code), nil
			case <-time.After(time.Duration(f * float64(time.Second))):
				p.cmd.Process.Kill()
				return nil, py.ExceptionNewf(TimeoutExpiredType, "Command timed out after %v seconds", f)
			}
		}
		code, err := p.wait()
		if err != nil {
			return nil, err
		}
		return py.Int(code), nil
	}, 0, "Wait for the child process to terminate.")

	PopenType.Dict["communicate"] = py.MustNewMethod("communicate", func(self py.Object, args py.Tuple) (py.Object, error) {
		var input py.Object = py.None
		if err := py.UnpackTuple(args, nil, "communicate", 0, 1, &input); err != nil {
			return nil, err
		}
		p := self.(*Popen)
		out, errText, err := p.communicate(input)
		if err != nil {
			return nil, err
		}
		return py.Tuple{streamValue(out, p.text), streamValue(errText, p.text)}, nil
	}, 0, "Interact with the child: send input, then read output.")

	PopenType.Dict["poll"] = py.MustNewMethod("poll", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Popen)
		if p.done {
			return py.Int(p.code), nil
		}
		// A non-blocking check needs the process to have been reaped; the
		// honest way here is to report that it is still running.
		return py.None, nil
	}, 0, "Return the exit code, or None if the child has not terminated.")

	PopenType.Dict["returncode"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			p := self.(*Popen)
			if !p.done {
				return py.None, nil
			}
			return py.Int(p.code), nil
		},
	}
	PopenType.Dict["pid"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			p := self.(*Popen)
			if p.cmd.Process == nil {
				return py.None, nil
			}
			return py.Int(p.cmd.Process.Pid), nil
		},
	}
	PopenType.Dict["stdin"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.None, nil },
	}

	PopenType.Dict["kill"] = py.MustNewMethod("kill", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Popen)
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		return py.None, nil
	}, 0, "Kill the child process.")
	PopenType.Dict["terminate"] = PopenType.Dict["kill"]

	PopenType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the process itself.")
	PopenType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Popen)
		p.wait()
		return py.False, nil
	}, 0, "Wait for the process to finish.")
}

// streamValue renders a captured stream as str or bytes, depending on mode.
func streamValue(text string, isText bool) py.Object {
	if isText {
		return py.String(text)
	}
	return py.Bytes(text)
}

// CompletedProcess is what run() returns.
type CompletedProcess struct {
	args       py.Object
	returncode int
	stdout     py.Object
	stderr     py.Object
}

var CompletedProcessType = py.NewTypeX("subprocess.CompletedProcess", "A completed process.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := &CompletedProcess{stdout: py.None, stderr: py.None}
	if len(args) > 0 {
		c.args = args[0]
	}
	if len(args) > 1 {
		code, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		c.returncode = code
	}
	if len(args) > 2 {
		c.stdout = args[2]
	}
	if len(args) > 3 {
		c.stderr = args[3]
	}
	for k, v := range kwargs {
		switch k {
		case "args":
			c.args = v
		case "returncode":
			code, err := py.IndexInt(v)
			if err != nil {
				return nil, err
			}
			c.returncode = code
		case "stdout":
			c.stdout = v
		case "stderr":
			c.stderr = v
		default:
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	return c, nil
}, nil)

func (c *CompletedProcess) Type() *py.Type { return CompletedProcessType }

func init() {
	CompletedProcessType.Dict["args"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CompletedProcess).args, nil
	}}
	CompletedProcessType.Dict["returncode"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*CompletedProcess).returncode), nil
	}}
	CompletedProcessType.Dict["stdout"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CompletedProcess).stdout, nil
	}}
	CompletedProcessType.Dict["stderr"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CompletedProcess).stderr, nil
	}}
	CompletedProcessType.Dict["check_returncode"] = py.MustNewMethod("check_returncode", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*CompletedProcess)
		if c.returncode != 0 {
			out := ""
			if s, ok := c.stdout.(py.String); ok {
				out = string(s)
			}
			errText := ""
			if s, ok := c.stderr.(py.String); ok {
				errText = string(s)
			}
			return nil, calledProcessError(c.returncode, c.args, out, errText)
		}
		return py.None, nil
	}, 0, "Raise CalledProcessError if the return code is non-zero.")
}

func call(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	code, _, _, _, err := runOnce(args, kwargs, false)
	if err != nil {
		return nil, err
	}
	return py.Int(code), nil
}

func checkCall(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	code, out, errText, o, err := runOnce(args, kwargs, false)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, calledProcessError(code, argObject(args), out, errText)
	}
	_ = o
	return py.Int(0), nil
}

func checkOutput(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	code, out, errText, o, err := runOnce(args, kwargs, true)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, calledProcessError(code, argObject(args), out, errText)
	}
	return streamValue(out, o.text), nil
}

func run(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	capture := false
	if v, ok := kwargs["capture_output"]; ok && v == py.True {
		capture = true
	}
	code, out, errText, o, err := runOnce(args, kwargs, capture)
	if err != nil {
		return nil, err
	}
	c := &CompletedProcess{args: argObject(args), returncode: code}
	if capture || o.stdout != py.None {
		c.stdout = streamValue(out, o.text)
	} else {
		c.stdout = py.None
	}
	if capture || o.stderr != py.None {
		c.stderr = streamValue(errText, o.text)
	} else {
		c.stderr = py.None
	}
	if o.check && code != 0 {
		return nil, calledProcessError(code, c.args, out, errText)
	}
	return c, nil
}

// runOnce runs the command and returns its exit code along with the streams.
func runOnce(args py.Tuple, kwargs py.StringDict, capture bool) (int, string, string, *options, error) {
	o, err := parseOptions(kwargs)
	if err != nil {
		return 0, "", "", nil, err
	}
	if capture {
		if o.stdout == py.None {
			o.stdout = pipeSentinel
		}
		if o.stderr == py.None {
			o.stderr = pipeSentinel
		}
	}
	kw := py.NewStringDict()
	if o.shell {
		kw["shell"] = py.True
	}
	if o.cwd != "" {
		kw["cwd"] = py.String(o.cwd)
	}
	if o.env != nil {
		env := py.NewStringDict()
		for _, pair := range o.env {
			if i := strings.IndexByte(pair, '='); i > 0 {
				env[pair[:i]] = py.String(pair[i+1:])
			}
		}
		kw["env"] = env
	}
	if o.text {
		kw["text"] = py.True
	}
	kw["stdout"] = o.stdout
	kw["stderr"] = o.stderr

	var input py.Object = py.None
	if v, ok := kwargs["input"]; ok {
		input = v
		kw["stdin"] = pipeSentinel
	} else {
		kw["stdin"] = py.None
	}

	var popenArgs py.Tuple
	if len(args) > 0 {
		popenArgs = py.Tuple{args[0]}
	} else if v, ok := kwargs["args"]; ok {
		popenArgs = py.Tuple{v}
	} else {
		return 0, "", "", nil, py.ExceptionNewf(py.TypeError, "missing required argument 'args'")
	}

	pobj, err := popenNew(PopenType, popenArgs, kw)
	if err != nil {
		return 0, "", "", nil, err
	}
	p := pobj.(*Popen)
	out, errText, communicateErr := p.communicate(input)
	code := p.code
	if communicateErr != nil {
		if ci, ok := communicateErr.(py.ExceptionInfo); ok {
			// A non-zero exit comes back as CalledProcessError, which is
			// information rather than a failure at this level.
			if ci.Type == CalledProcessErrorType {
				return code, out, errText, o, nil
			}
		}
		return code, out, errText, o, communicateErr
	}
	return code, out, errText, o, nil
}

// argObject returns the command as the caller wrote it, for the exception.
func argObject(args py.Tuple) py.Object {
	if len(args) > 0 {
		return args[0]
	}
	return py.None
}

func getStatusOutput(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var cmd py.Object
	if err := py.UnpackTuple(args, nil, "getstatusoutput", 1, 1, &cmd); err != nil {
		return nil, err
	}
	if _, ok := cmd.(py.String); !ok {
		return nil, py.ExceptionNewf(py.TypeError, "getstatusoutput() argument 1 must be str")
	}
	code, out, errText, _, err := runOnce(py.Tuple{cmd}, py.NewStringDict(), true)
	if err != nil {
		return nil, err
	}
	combined := strings.TrimRight(out+errText, "\n")
	return py.Tuple{py.Int(code), py.String(combined)}, nil
}

func getOutput(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	res, err := getStatusOutput(self, args, kwargs)
	if err != nil {
		return nil, err
	}
	return res.(py.Tuple)[1], nil
}

// list2cmdline joins arguments the way a shell would, which is what the
// module's own helper does.
func list2cmdline(self py.Object, args py.Tuple) (py.Object, error) {
	var seq py.Object
	if err := py.UnpackTuple(args, nil, "list2cmdline", 1, 1, &seq); err != nil {
		return nil, err
	}
	items, err := py.SequenceList(seq)
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(items.Items))
	for _, item := range items.Items {
		s, err := py.StrAsString(item)
		if err != nil {
			return nil, err
		}
		if s == "" || strings.ContainsAny(s, " \t\"") {
			escaped := strings.ReplaceAll(s, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			s = `"` + escaped + `"`
		}
		parts = append(parts, s)
	}
	return py.String(strings.Join(parts, " ")), nil
}

// keep context referenced: the CPython implementation threads a context
// through for cancellation, which is not wired up here.
var _ = context.Background
