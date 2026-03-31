package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"t2f/codegen"
	"t2f/lexer"
	"t2f/parser"
)

type compileOptions struct {
	target codegen.Target
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "compile":
		filename, opts := parseCommandArgs(os.Args[2:])
		compileFile(filename, opts)
	case "run":
		filename, opts := parseCommandArgs(os.Args[2:])
		binName := compileFile(filename, opts)
		runBinary(binName)
	case "repl":
		_, opts := parseCommandArgs(os.Args[2:])
		runRepl(opts)
	case "install":
		handleInstall(os.Args[2:])
	case "update":
		handleUpdate(os.Args[2:])
	case "list":
		handleList(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  t2f compile [--target linux|windows] <file.t2f>")
	fmt.Println("  t2f run [--target linux|windows] <file.t2f>")
	fmt.Println("  t2f repl [--target linux|windows]")
	fmt.Println("  t2f install <path-or-git-url>")
	fmt.Println("  t2f update [package-name]")
	fmt.Println("  t2f list")
}

func parseCommandArgs(args []string) (string, compileOptions) {
	opts := compileOptions{target: codegen.DefaultTarget()}
	filename := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--target":
			if i+1 >= len(args) {
				fmt.Println("missing value for --target")
				os.Exit(1)
			}
			target, err := codegen.ParseTarget(args[i+1])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			opts.target = target
			i++
		case strings.HasPrefix(arg, "--target="):
			target, err := codegen.ParseTarget(strings.TrimPrefix(arg, "--target="))
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			opts.target = target
		default:
			if filename == "" {
				filename = arg
				continue
			}
			fmt.Printf("unexpected argument: %s\n", arg)
			os.Exit(1)
		}
	}

	return filename, opts
}

func runRepl(opts compileOptions) {
	if opts.target == codegen.TargetWindows {
		fmt.Println("t2f REPL is not supported on the Windows backend yet")
		return
	}

	fmt.Println("t2f REPL - ctrl+c to exit")
	history := []string{}
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("t2f> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "clear" {
			history = []string{}
			fmt.Println("// scope cleared")
			continue
		}

		history = append(history, line)
		fullSource := strings.Join(history, "\n")

		l := lexer.New(fullSource)
		p := parser.New(l, ".")
		program := p.ParseProgram()

		if len(p.Errors()) != 0 {
			history = history[:len(history)-1]
			for _, msg := range p.Errors() {
				fmt.Printf("  parse error: %s\n", msg)
			}
			continue
		}

		cg := codegen.NewForTarget(opts.target)
		err = cg.Compile(program)
		if err != nil {
			history = history[:len(history)-1]
			fmt.Printf("  error: %v\n", err)
			continue
		}

		asmCode := cg.Assemble()
		asmPath := "/tmp/t2f_repl.asm"
		objPath := "/tmp/t2f_repl.o"
		binPath := "/tmp/t2f_repl"

		os.WriteFile(asmPath, []byte(asmCode), 0644)

		nasmCmd := exec.Command("nasm", "-f", opts.target.ObjectFormat(), asmPath, "-o", objPath)
		if out, err := nasmCmd.CombinedOutput(); err != nil {
			history = history[:len(history)-1]
			fmt.Printf("  asm error: %s\n", string(out))
			continue
		}

		ldCmd := exec.Command("ld", objPath, "-o", binPath)
		if out, err := ldCmd.CombinedOutput(); err != nil {
			history = history[:len(history)-1]
			fmt.Printf("  link error: %s\n", string(out))
			continue
		}

		cmd := exec.Command(binPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

func compileFile(filename string, opts compileOptions) string {
	execName, err := compileFileE(filename, opts)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return execName
}

func compileFileE(filename string, opts compileOptions) (string, error) {
	if filename == "" {
		return "", errors.New("missing input file")
	}

	input, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	dir := filepath.Dir(filename)
	if dir == "." || dir == "" {
		dir = "."
	}

	l := lexer.New(string(input))
	p := parser.New(l, dir)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		var builder strings.Builder
		builder.WriteString("Parser Errors:\n")
		for _, msg := range p.Errors() {
			builder.WriteString("\t")
			builder.WriteString(msg)
			builder.WriteString("\n")
		}
		return "", errors.New(strings.TrimRight(builder.String(), "\n"))
	}

	cg := codegen.NewForTarget(opts.target)
	err = cg.Compile(program)
	if err != nil {
		return "", fmt.Errorf("Codegen Error: %w", err)
	}

	asmCode := cg.Assemble()

	baseName := strings.TrimSuffix(filename, ".t2f")
	asmFilename := baseName + ".asm"
	objFilename := baseName + ".o"
	execName := opts.target.ExecutableName(baseName)

	err = os.WriteFile(asmFilename, []byte(asmCode), 0644)
	if err != nil {
		return "", fmt.Errorf("error writing assembly: %w", err)
	}

	nasmCmd := exec.Command("nasm", "-f", opts.target.ObjectFormat(), asmFilename, "-o", objFilename)
	nasmOut, err := nasmCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Assembler (nasm) error: %v\nOutput: %s\nMake sure nasm is installed and on PATH.", err, string(nasmOut))
	}

	linkOut, err := linkObject(objFilename, execName, opts.target)
	if err != nil {
		if opts.target == codegen.TargetWindows {
			return "", fmt.Errorf("Linker error: %v\nOutput: %s\nInstall a Windows-capable linker such as gcc or clang and ensure kernel32 is available.", err, string(linkOut))
		}
		return "", fmt.Errorf("Linker error: %v\nOutput: %s", err, string(linkOut))
	}

	return execName, nil
}

func linkObject(objFilename, execName string, target codegen.Target) ([]byte, error) {
	if target == codegen.TargetWindows {
		for _, linker := range []string{"gcc", "clang"} {
			if _, err := exec.LookPath(linker); err == nil {
				cmd := exec.Command(linker, objFilename, "-o", execName, "-lkernel32")
				return cmd.CombinedOutput()
			}
		}
		return nil, fmt.Errorf("no supported Windows linker found on PATH")
	}

	cmd := exec.Command("ld", objFilename, "-o", execName)
	return cmd.CombinedOutput()
}

func runBinary(binName string) {
	cmd := exec.Command(binName)
	if filepath.Ext(binName) == "" {
		cmd = exec.Command("." + string(os.PathSeparator) + binName)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("Program exited with error: %v\n", err)
	}
}
