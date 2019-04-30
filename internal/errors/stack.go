package errors

import (
	"bytes"
	"strconv"
)

// StackFunc is a function call in the backtrace
type StackFunc struct {
	Name    string  `json:"name"`
	ArgPtrs []int64 `json:"name,omitempty"`
}

// StackElement represents a stack call including file, line, and function call
type StackElement struct {
	File   string `json:"file"`
	Line   int64  `json:"line"`
	Callee string `json:"calee,omitempty"`
	Addr   int64  `json:"address,omitempty"`
}
// Stack is an array of stack elements representing the parsed relevant bits of a backtrace
// Relevant in this context means, it strips the calls that are happening in the package
type Stack []StackElement

func parseStack(b []byte) (*Stack, error) {
	lvl := 2 // go up the stack call tree to hide the two internal calls
	lines := bytes.Split(b, []byte("\n"))

	if len(lines) <= lvl*2+1 {
		return nil, New("invalid stack trace")
	}

	skipLines := lvl * 2
	stackLen := (len(lines) - 1 - skipLines) / 2
	relLines := lines[1+skipLines:]

	stack := make(Stack, stackLen)
	for i, curLine := range relLines {
		cur := i / 2
		if len(curLine) == 0 {
			continue
		}
		curStack := stack[cur]
		if i%2 == 0 {
			// function line
			curStack.Callee = string(curLine)
			//elems := bytes.Split(curLine, []byte("("))
			//curStack.Callee.Name = string(elems[0])
			//argsLine := bytes.Trim(elems[1], ")")
			//args := bytes.Split(argsLine, []byte(","))
			//curStack.Callee.ArgPtrs = make([]int64, len(args))
			//for j, arg := range args {
			//	curStack.Callee.ArgPtrs[j], _ = strconv.ParseInt(string(bytes.Trim(arg, " ")), 16, 64)
			//}
		} else {
			// file line
			curLine = bytes.Trim(curLine, "\t")
			elems := bytes.Split(curLine, []byte(":"))
			curStack.File = string(elems[0])

			elems1 := bytes.Split(elems[1], []byte(" "))
			cnt := len(elems1)
			if cnt > 0 {
				curStack.Line, _ = strconv.ParseInt(string(elems1[0]), 10, 64)
			}
			if cnt > 1 {
				curStack.Addr, _ = strconv.ParseInt(string(elems1[1]), 16, 64)
			}
		}
		stack[cur] = curStack
	}
	return &stack, nil
}
