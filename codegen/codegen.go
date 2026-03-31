package codegen

import (
	"bytes"
	"fmt"
	"t2f/ast"
)

type Compiler struct {
	assembly    bytes.Buffer
	dataSection bytes.Buffer
	bssSection  bytes.Buffer
	textSection bytes.Buffer
	target      Target

	// Map of variable names to their rbp offset
	locals             map[string]int
	globals            map[string]string
	currentOffset      int
	labelCounter       int
	currentFunctionEnd string
}

func New() *Compiler {
	return NewForTarget(DefaultTarget())
}

func NewForTarget(target Target) *Compiler {
	return &Compiler{
		locals:             make(map[string]int),
		globals:            make(map[string]string),
		currentOffset:      0,
		labelCounter:       0,
		currentFunctionEnd: "",
		target:             target,
	}
}

func (c *Compiler) Compile(node ast.Node) error {
	switch n := node.(type) {
	case *ast.Program:
		for _, stmt := range n.Statements {
			err := c.Compile(stmt)
			if err != nil {
				return err
			}
		}

	case *ast.ExpressionStatement:
		return c.Compile(n.Expression)

	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			err := c.Compile(stmt)
			if err != nil {
				return err
			}
		}

	case *ast.AssignStatement:
		err := c.Compile(n.Value)
		if err != nil {
			return err
		}

		// 1. Check if it's a global
		if label, isGlobal := c.globals[n.Name.Value]; isGlobal {
			c.textSection.WriteString(fmt.Sprintf("    mov [%s], rax\n", label))
			return nil
		}

		// 2. Check if it's a local
		offset, ok := c.locals[n.Name.Value]
		if !ok {
			return fmt.Errorf("cannot assign to undeclared variable: %s", n.Name.Value)
		}
		c.textSection.WriteString(fmt.Sprintf("    mov [rbp - %d], rax\n", offset))
	// arr[i] = val
	case *ast.IndexAssignStatement:
		// 1. Compile value
		err := c.Compile(n.Value)
		if err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n") // save value

		// 2. Pull the index expression apart
		idxExpr, ok := n.Left.(*ast.IndexExpression)
		if !ok {
			return fmt.Errorf("left side of index assign is not an index expression")
		}

		// 3. Compile base pointer
		err = c.Compile(idxExpr.Left)
		if err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n") // save base

		// 4. Compile index
		err = c.Compile(idxExpr.Index)
		if err != nil {
			return err
		}

		c.textSection.WriteString("    shl rax, 3\n") // index * 8 (8-byte slots)
		c.textSection.WriteString("    add rax, 8\n")
		c.textSection.WriteString("    pop rbx\n")      // base
		c.textSection.WriteString("    add rbx, rax\n") // address = base + index*8
		c.textSection.WriteString("    pop rax\n")      // value
		c.textSection.WriteString("    mov [rbx], rax\n")

	case *ast.WhileStatement:
		startLabel := fmt.Sprintf(".loop_start_%d", c.labelCounter)
		endLabel := fmt.Sprintf(".loop_end_%d", c.labelCounter)
		c.labelCounter++

		c.textSection.WriteString(fmt.Sprintf("%s:\n", startLabel))

		err := c.Compile(n.Condition)
		if err != nil {
			return err
		}

		c.textSection.WriteString("    test rax, rax\n")
		c.textSection.WriteString(fmt.Sprintf("    jz %s\n", endLabel))

		err = c.Compile(n.Body)
		if err != nil {
			return err
		}

		c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", startLabel))
		c.textSection.WriteString(fmt.Sprintf("%s:\n", endLabel))

	case *ast.IfExpression:
		elseLabel := fmt.Sprintf(".else_branch_%d", c.labelCounter)
		endLabel := fmt.Sprintf(".end_if_%d", c.labelCounter)
		c.labelCounter++

		err := c.Compile(n.Condition)
		if err != nil {
			return err
		}

		c.textSection.WriteString("    test rax, rax\n")
		c.textSection.WriteString(fmt.Sprintf("    jz %s\n", elseLabel))

		err = c.Compile(n.Consequence)
		if err != nil {
			return err
		}
		c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", endLabel))

		c.textSection.WriteString(fmt.Sprintf("%s:\n", elseLabel))
		if n.Alternative != nil {
			err = c.Compile(n.Alternative)
			if err != nil {
				return err
			}
		}

		c.textSection.WriteString(fmt.Sprintf("%s:\n", endLabel))
	case *ast.ArrayLiteral:
		if c.target == TargetWindows {
			return c.compileWindowsArrayLiteral(n)
		}
		count := len(n.Elements)
		size := count*8 + 8

		c.textSection.WriteString(fmt.Sprintf("    mov rsi, %d\n", size))
		c.textSection.WriteString("    mov rdi, 0\n")
		c.textSection.WriteString("    mov rdx, 3\n")
		c.textSection.WriteString("    mov r10, 34\n")
		c.textSection.WriteString("    mov r8, -1\n")
		c.textSection.WriteString("    mov r9, 0\n")
		c.textSection.WriteString("    mov rax, 9\n")
		c.textSection.WriteString("    syscall\n")

		c.textSection.WriteString(fmt.Sprintf("    mov qword [rax], %d\n", count))
		c.textSection.WriteString("    push rax\n")

		for i, elem := range n.Elements {
			err := c.Compile(elem)
			if err != nil {
				return err
			}
			offset := 8 + i*8
			c.textSection.WriteString("    mov rcx, [rsp]\n")
			c.textSection.WriteString(fmt.Sprintf("    mov [rcx + %d], rax\n", offset))
		}

		c.textSection.WriteString("    pop rax\n")
	case *ast.CallExpression:
		if c.target == TargetWindows {
			return c.compileWindowsCallExpression(n)
		}
		var funcName string
		if ident, ok := n.Function.(*ast.Identifier); ok {
			funcName = ident.Value
		}

		if funcName == "sys_argc" {
			// sys_argc() -> returns the number of arguments
			c.textSection.WriteString("    mov rax, [sys_argc_val]\n")
			return nil
		}
		if funcName == "sys_fork" {
			// sys_fork() -> returns 0 in the child, PID in the parent
			c.textSection.WriteString("    mov rax, 57\n    syscall\n")
			return nil
		}

		if funcName == "fs_read_to" {
			// fs_read_to(fd, ptr, len)
			c.Compile(n.Arguments[2])
			c.textSection.WriteString("    push rax\n") // len
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n") // ptr
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rsi\n")      // ptr -> rsi
			c.textSection.WriteString("    pop rdx\n")      // len -> rdx
			c.textSection.WriteString("    mov rdi, rax\n") // fd -> rdi
			c.textSection.WriteString("    mov rax, 0\n")   // sys_read
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "set_byte" {
			// set_byte(ptr, index, val)
			c.Compile(n.Arguments[2])
			c.textSection.WriteString("    push rax\n")
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n")
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rbx\n    pop rcx\n")
			c.textSection.WriteString("    add rax, rbx\n")
			c.textSection.WriteString("    mov [rax], cl\n")
			return nil
		}
		if funcName == "sys_wait" {
			c.textSection.WriteString("    mov rdi, -1\n") // Wait for any child
			c.textSection.WriteString("    mov rsi, 0\n")  // Ignore status
			c.textSection.WriteString("    mov rdx, 0\n")  // No options
			c.textSection.WriteString("    mov r10, 0\n")  // No rusage
			c.textSection.WriteString("    mov rax, 61\n") // sys_wait4
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "sys_exec" {
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n") // argv
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    mov rdi, rax\n") // path
			c.textSection.WriteString("    pop rsi\n")      // argv

			// THE FIX: Linux expects RDX to point to a NULL pointer, not just be 0.
			c.textSection.WriteString("    push 0\n")
			c.textSection.WriteString("    mov rdx, rsp\n")

			c.textSection.WriteString("    mov rax, 59\n") // execve
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    add rsp, 8\n")
			return nil
		}
		if funcName == "sys_argv" {
			// sys_argv(index) -> returns the pointer to that string
			err := c.Compile(n.Arguments[0]) // Get index
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rbx, [sys_argv_ptr]\n")
			c.textSection.WriteString("    shl rax, 3\n")     // index * 8
			c.textSection.WriteString("    add rbx, rax\n")   // argv + (index * 8)
			c.textSection.WriteString("    mov rax, [rbx]\n") // load the pointer to the string
			return nil
		}
		if funcName == "exit" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 60\n") // sys_exit
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "sys_exit" {
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 60\n") // sys_exit
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "net_on_interrupt" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; setup SIGINT handler\n")
			c.textSection.WriteString("    mov [sigint_actor_pipe], rax\n")

			c.textSection.WriteString("    sub rsp, 32\n")
			c.textSection.WriteString("    mov qword [rsp], sigint_stub\n")
			c.textSection.WriteString("    mov qword [rsp+8], 0x04000000\n")
			c.textSection.WriteString("    mov qword [rsp+16], sig_restorer_stub\n")
			c.textSection.WriteString("    mov qword [rsp+24], 0\n")

			c.textSection.WriteString("    mov rdi, 2\n")
			c.textSection.WriteString("    mov rsi, rsp\n")
			c.textSection.WriteString("    mov rdx, 0\n")
			c.textSection.WriteString("    mov r10, 8\n")
			c.textSection.WriteString("    mov rax, 13\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    add rsp, 32\n")
			return nil
		}

		if funcName == "net_error" {
			c.textSection.WriteString("    ; net_error\n")
			c.textSection.WriteString("    mov rax, 0\n")
			c.textSection.WriteString("    mov rax, qword [errno]\n")
			return nil
		}
		if funcName == "int_to_buf" {
			err := c.Compile(n.Arguments[1])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    pop rsi\n")
			c.textSection.WriteString("    call int_to_buf\n")
			return nil
		}
		if funcName == "sys_getdents" {
			// sys_getdents(fd, buf, size) -> returns number of bytes read
			c.Compile(n.Arguments[2])
			c.textSection.WriteString("    push rax\n") // size
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n") // buf
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rsi\n")      // buf
			c.textSection.WriteString("    pop rdx\n")      // size
			c.textSection.WriteString("    mov rdi, rax\n") // fd
			c.textSection.WriteString("    mov rax, 217\n") // sys_getdents64
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "sys_mkdir" {
			// sys_mkdir(path, mode)
			// mode is usually 493 (which is 0755 in octal)
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n") // mode
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    mov rdi, rax\n") // path
			c.textSection.WriteString("    pop rsi\n")      // mode
			c.textSection.WriteString("    mov rax, 83\n")  // sys_mkdir
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "sys_unlink" {
			// sys_unlink(path) - deletes a file
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 87\n") // sys_unlink
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}
		if funcName == "get_uint16" {
			// get_uint16(ptr, offset) -> used to read the 'record length'
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n")
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rbx\n")
			c.textSection.WriteString("    add rax, rbx\n")
			c.textSection.WriteString("    movzx rax, word [rax]\n")
			return nil
		}

		if funcName == "get_byte" {
			// get_byte(ptr, offset) -> used for general byte-level parsing
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n")
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rbx\n")
			c.textSection.WriteString("    add rax, rbx\n")
			c.textSection.WriteString("    movzx rax, byte [rax]\n")
			return nil
		}
		if funcName == "mem_set" {
			// mem_set(ptr, index, value)
			err := c.Compile(n.Arguments[2])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // value
			err = c.Compile(n.Arguments[1])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // index
			err = c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}

			c.textSection.WriteString("    pop rbx\n")    // index
			c.textSection.WriteString("    pop rcx\n")    // value
			c.textSection.WriteString("    shl rbx, 3\n") // index * 8
			c.textSection.WriteString("    add rax, rbx\n")
			c.textSection.WriteString("    mov [rax], rcx\n")
			return nil
		}

		if funcName == "mem_get" {
			// mem_get(ptr, index) -> rax
			err := c.Compile(n.Arguments[1])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // index
			err = c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}

			c.textSection.WriteString("    pop rbx\n")    // index
			c.textSection.WriteString("    shl rbx, 3\n") // index * 8
			c.textSection.WriteString("    add rax, rbx\n")
			c.textSection.WriteString("    mov rax, [rax]\n")
			return nil
		}

		if funcName == "mem_alloc" {
			// mem_alloc(size_in_bytes) -> pointer in rax
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}

			c.textSection.WriteString("    ; sys_mmap(addr=0, len=rax, prot=3, flags=34, fd=-1, offset=0)\n")
			c.textSection.WriteString("    mov rsi, rax\n")
			c.textSection.WriteString("    mov rdi, 0\n")
			c.textSection.WriteString("    mov rdx, 3\n")  // PROT_READ | PROT_WRITE
			c.textSection.WriteString("    mov r10, 34\n") // MAP_PRIVATE | MAP_ANONYMOUS
			c.textSection.WriteString("    mov r8, -1\n")
			c.textSection.WriteString("    mov r9, 0\n")
			c.textSection.WriteString("    mov rax, 9\n") // sys_mmap
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "array" {
			err := c.Compile(n.Arguments[0]) // n = element count
			if err != nil {
				return err
			}
			// allocate (n+1)*8 bytes — extra slot at front for length
			c.textSection.WriteString("    push rax\n")           // save count
			c.textSection.WriteString("    lea rsi, [rax*8+8]\n") // (n+1)*8
			c.textSection.WriteString("    mov rdi, 0\n")
			c.textSection.WriteString("    mov rdx, 3\n")
			c.textSection.WriteString("    mov r10, 34\n")
			c.textSection.WriteString("    mov r8, -1\n")
			c.textSection.WriteString("    mov r9, 0\n")
			c.textSection.WriteString("    mov rax, 9\n") // sys_mmap
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    pop rbx\n")        // restore count
			c.textSection.WriteString("    mov [rax], rbx\n") // write length at ptr[0]
			return nil
		}
		if funcName == "mem_free" {
			err := c.Compile(n.Arguments[0]) // ptr
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			if len(n.Arguments) > 1 {
				err = c.Compile(n.Arguments[1]) // explicit size in bytes for raw mem_alloc blocks
				if err != nil {
					return err
				}
				c.textSection.WriteString("    mov rsi, rax\n")
			} else {
				c.textSection.WriteString("    mov rsi, [rdi]\n")     // read length from header
				c.textSection.WriteString("    lea rsi, [rsi*8+8]\n") // convert to bytes
			}
			c.textSection.WriteString("    mov rax, 11\n") // sys_munmap
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "str_to_int" {
			// str_to_int(ptr) -> returns the number in rax
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    call asm_str_to_int\n")
			return nil
		}
		if funcName == "str_len" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n") // string pointer
			c.textSection.WriteString("    mov rcx, 0\n")
			strLenLabel := fmt.Sprintf(".str_len_loop_%d", c.labelCounter)
			strLenDone := fmt.Sprintf(".str_len_done_%d", c.labelCounter)
			c.labelCounter++
			c.textSection.WriteString(fmt.Sprintf("%s:\n", strLenLabel))
			c.textSection.WriteString("    cmp byte [rdi + rcx], 0\n")
			c.textSection.WriteString(fmt.Sprintf("    je %s\n", strLenDone))
			c.textSection.WriteString("    inc rcx\n")
			c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", strLenLabel))
			c.textSection.WriteString(fmt.Sprintf("%s:\n", strLenDone))
			c.textSection.WriteString("    mov rax, rcx\n")
			return nil
		}
		if funcName == "disk_total" || funcName == "disk_avail" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, statfs_buf\n")
			c.textSection.WriteString("    mov rax, 137\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			c.textSection.WriteString("    cmp rax, -1\n")
			failLabel := fmt.Sprintf(".disk_statfs_fail_%d", c.labelCounter)
			doneLabel := fmt.Sprintf(".disk_statfs_done_%d", c.labelCounter)
			c.labelCounter++
			c.textSection.WriteString(fmt.Sprintf("    je %s\n", failLabel))
			if funcName == "disk_total" {
				c.textSection.WriteString("    mov rax, [statfs_buf + 16]\n")
			} else {
				c.textSection.WriteString("    mov rax, [statfs_buf + 32]\n")
			}
			c.textSection.WriteString("    mov rbx, [statfs_buf + 72]\n")
			c.textSection.WriteString("    test rbx, rbx\n")
			c.textSection.WriteString(fmt.Sprintf("    jnz %s\n", doneLabel))
			c.textSection.WriteString("    mov rbx, [statfs_buf + 8]\n")
			c.textSection.WriteString(fmt.Sprintf("%s:\n", doneLabel))
			c.textSection.WriteString("    imul rax, rbx\n")
			endLabel := fmt.Sprintf(".disk_statfs_end_%d", c.labelCounter)
			c.labelCounter++
			c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", endLabel))
			c.textSection.WriteString(fmt.Sprintf("%s:\n", failLabel))
			c.textSection.WriteString("    mov rax, -1\n")
			c.textSection.WriteString(fmt.Sprintf("%s:\n", endLabel))
			return nil
		}

		if funcName == "net_match_path" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rsi, rax\n")
			c.textSection.WriteString("    mov rdi, net_request_buf\n")
			c.textSection.WriteString("    call find_path_in_request\n")
			return nil
		}
		if funcName == "net_path_eq" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rsi, rax\n")
			c.textSection.WriteString("    mov rdi, net_request_buf\n")
			c.textSection.WriteString("    call request_path_eq\n")
			return nil
		}
		if funcName == "net_copy_path_tail" {
			err := c.Compile(n.Arguments[1])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    pop rdx\n")
			c.textSection.WriteString("    mov rsi, rax\n")
			c.textSection.WriteString("    mov rdi, net_request_buf\n")
			c.textSection.WriteString("    call copy_request_path_tail\n")
			return nil
		}

		if funcName == "net_send_status_ok" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")

			strLabel := fmt.Sprintf("http_ok_%d", c.labelCounter)
			c.labelCounter++
			c.dataSection.WriteString(fmt.Sprintf("    %s db `HTTP/1.1 200 OK\\r\\nContent-Type: text/html\\r\\nConnection: close\\r\\n\\r\\n`, 0\n", strLabel))

			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString(fmt.Sprintf("    mov rsi, %s\n", strLabel))
			c.textSection.WriteString("    mov rdx, 62\n")
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "net_send_int" {
			// net_send_int(client_fd, number)
			// Compile fd first, save it, then convert number to string
			err := c.Compile(n.Arguments[0]) // fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // save fd

			err = c.Compile(n.Arguments[1]) // number into rax
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, net_scratch_buf\n")
			c.textSection.WriteString("    call num_to_str\n") // rsi=buf, rdx=len
			c.textSection.WriteString("    pop rdi\n")         // restore fd
			c.textSection.WriteString("    mov rax, 1\n")      // sys_write
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "net_write" {
			// net_write(fd, ptr, len)
			err := c.Compile(n.Arguments[2]) // len
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[1]) // ptr
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0]) // fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n") // fd
			c.textSection.WriteString("    pop rsi\n")      // ptr
			c.textSection.WriteString("    pop rdx\n")      // len
			c.textSection.WriteString("    mov rax, 1\n")   // sys_write
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "str_cmp" {
			// str_cmp(s1, s2) -> returns 1 if equal, 0 if not
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n")
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rsi\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    call asm_str_cmp\n")
			return nil
		}
		if funcName == "print_str" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_write string\n")
			c.textSection.WriteString("    call print_str\n")
			return nil
		}

		if funcName == "write_str" {
			// write_str(fd, ptr) — writes null-terminated string to any fd
			err := c.Compile(n.Arguments[1]) // ptr
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0]) // fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")      // fd
			c.textSection.WriteString("    pop rax\n")           // ptr back into rax
			c.textSection.WriteString("    call write_str_fd\n") // reuse the strlen+write routine
			// note: print_str currently hardcodes fd=1, so you'd want a new routine here
			return nil
		}

		if funcName == "net_socket" {
			c.textSection.WriteString("    ; sys_socket\n")
			c.textSection.WriteString("    mov rdi, 2\n") // AF_INET
			c.textSection.WriteString("    mov rsi, 1\n") // SOCK_STREAM
			c.textSection.WriteString("    mov rdx, 0\n")
			c.textSection.WriteString("    mov rax, 41\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}
		if funcName == "net_reuse_addr" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, 1\n")
			c.textSection.WriteString("    mov rdx, 2\n")
			c.textSection.WriteString("    push 1\n")
			c.textSection.WriteString("    mov r10, rsp\n")
			c.textSection.WriteString("    mov r8, 4\n")
			c.textSection.WriteString("    mov rax, 54\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    add rsp, 8\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "net_bind" {
			if len(n.Arguments) == 3 {
				err := c.Compile(n.Arguments[2])
				if err != nil {
					return err
				}
				c.textSection.WriteString("    push rax\n")
				err = c.Compile(n.Arguments[1])
				if err != nil {
					return err
				}
			} else {
				if len(n.Arguments) > 1 {
					err := c.Compile(n.Arguments[1])
					if err != nil {
						return err
					}
				} else {
					c.textSection.WriteString("    mov rax, 8080\n")
				}
				c.textSection.WriteString("    push rax\n")
				c.textSection.WriteString("    mov rax, bind_any_addr\n")
			}
			c.textSection.WriteString("    push rax\n")

			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}

			c.textSection.WriteString("    push rax\n")
			c.textSection.WriteString("    sub rsp, 16\n")
			c.textSection.WriteString("    mov word [rsp], 2\n")
			c.textSection.WriteString("    mov ax, [rsp + 32]\n")
			c.textSection.WriteString("    xchg al, ah\n")
			c.textSection.WriteString("    mov [rsp + 2], ax\n")
			c.textSection.WriteString("    mov rdi, [rsp + 24]\n")
			c.textSection.WriteString("    call parse_bind_host\n")
			c.textSection.WriteString("    mov [rsp + 4], eax\n")
			c.textSection.WriteString("    mov qword [rsp + 8], 0\n")
			c.textSection.WriteString("    mov rdi, [rsp + 16]\n")
			c.textSection.WriteString("    mov rsi, rsp\n")
			c.textSection.WriteString("    mov rdx, 16\n")
			c.textSection.WriteString("    mov rax, 49\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			c.textSection.WriteString("    add rsp, 40\n")
			return nil
		}

		if funcName == "net_listen" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_listen\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 50\n")
			c.textSection.WriteString("    mov rsi, 10\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "net_accept" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_accept\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 43\n")
			c.textSection.WriteString("    mov rsi, 0\n")
			c.textSection.WriteString("    mov rdx, 0\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "net_read" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_read network drain\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 0\n")
			c.textSection.WriteString("    mov rsi, net_request_buf\n")
			c.textSection.WriteString("    mov rdx, 4095\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			doneLabel := fmt.Sprintf(".net_read_done_%d", c.labelCounter)
			c.labelCounter++
			c.textSection.WriteString("    cmp rax, 0\n")
			c.textSection.WriteString(fmt.Sprintf("    jl %s\n", doneLabel))
			c.textSection.WriteString("    mov rcx, net_request_buf\n")
			c.textSection.WriteString("    add rcx, rax\n")
			c.textSection.WriteString("    mov byte [rcx], 0\n")
			c.textSection.WriteString(fmt.Sprintf("%s:\n", doneLabel))
			return nil
		}

		if funcName == "net_respond" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			strLabel := fmt.Sprintf("http_%d", c.labelCounter)
			c.labelCounter++

			payload := "HTTP/1.1 200 OK\\r\\nContent-Type: text/html\\r\\nContent-Length: 124\\r\\n\\r\\n<html><body style='font-family:sans-serif'><h1>Welcome to t2f!</h1><p>Served natively from a Kernel Actor!</p></body></html>"
			c.dataSection.WriteString(fmt.Sprintf("    %s db `%s`, 0\n", strLabel, payload))

			c.textSection.WriteString("    ; sys_write HTTP response\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString(fmt.Sprintf("    mov rsi, %s\n", strLabel))
			c.textSection.WriteString("    mov rdx, 189\n")
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "net_close" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_close\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 3\n")
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "fs_open" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_open(readonly)\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, 0\n")
			c.textSection.WriteString("    mov rdx, 0\n")
			c.textSection.WriteString("    mov rax, 2\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "fs_open_write" {
			// fs_open_write(path_string)
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, 577\n") // O_WRONLY | O_CREAT | O_TRUNC
			c.textSection.WriteString("    mov rdx, 420\n") // 0644 permissions
			c.textSection.WriteString("    mov rax, 2\n")   // sys_open
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "fs_write" {
			// fs_write(fd, ptr, len)
			c.Compile(n.Arguments[2])
			c.textSection.WriteString("    push rax\n") // len
			c.Compile(n.Arguments[1])
			c.textSection.WriteString("    push rax\n") // ptr
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    pop rsi\n")      // ptr -> rsi
			c.textSection.WriteString("    pop rdx\n")      // len -> rdx
			c.textSection.WriteString("    mov rdi, rax\n") // fd -> rdi
			c.textSection.WriteString("    mov rax, 1\n")   // sys_write
			c.textSection.WriteString("    syscall\n")
			return nil
		}

		if funcName == "fs_read" {
			err := c.Compile(n.Arguments[1]) // count
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0]) // fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    pop rdx\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, file_io_buf\n")
			c.textSection.WriteString("    mov rax, 0\n")
			c.textSection.WriteString("    syscall\n")
			c.textSection.WriteString("    call check_rax\n")
			return nil
		}

		if funcName == "fs_close" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    ; sys_close\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rax, 3\n")
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		/*	if funcName == "sys_exec" {
			// sys_exec(path_string, argv_pointer)
			// 1. Compile the argv pointer (the array of pointers to strings)
			err := c.Compile(n.Arguments[1])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // Save argv_ptr

			// 2. Compile the path string pointer (e.g., "/bin/ls")
			err = c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}

			c.textSection.WriteString("    mov rdi, rax\n") // rdi = filename path
			c.textSection.WriteString("    pop rsi\n")      // rsi = argv array pointer
			c.textSection.WriteString("    mov rdx, 0\n")   // rdx = envp (NULL for simplicity)
			c.textSection.WriteString("    mov rax, 59\n")  // sys_execve
			c.textSection.WriteString("    syscall\n")
			return nil
		}*/

		if funcName == "net_send_buf" {
			err := c.Compile(n.Arguments[1]) // size
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
			err = c.Compile(n.Arguments[0]) // fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    pop rdx\n")
			c.textSection.WriteString("    mov rdi, rax\n")
			c.textSection.WriteString("    mov rsi, file_io_buf\n")
			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString("    syscall\n")
			return nil
		}
		if funcName == "is_alpha" {
			err := c.Compile(n.Arguments[0])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    call asm_is_alpha\n")
			return nil
		}
		if funcName == "is_digit" {
			c.Compile(n.Arguments[0])
			c.textSection.WriteString("    call asm_is_digit\n")
			return nil
		}
		if funcName == "net_respond_headers" {
			// net_respond_headers(client_fd, body_size)
			err := c.Compile(n.Arguments[1]) // body_size
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // save body_size

			err = c.Compile(n.Arguments[0]) // client_fd
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n") // save client_fd

			headerPrefixLabel := fmt.Sprintf("http_prefix_%d", c.labelCounter)
			c.labelCounter++
			prefix := "HTTP/1.1 200 OK\\r\\nContent-Type: text/html\\r\\nContent-Length: "
			c.dataSection.WriteString(fmt.Sprintf("    %s db `%s`, 0\n", headerPrefixLabel, prefix))

			c.textSection.WriteString("    mov rdi, [rsp]\n")
			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString(fmt.Sprintf("    mov rsi, %s\n", headerPrefixLabel))
			c.textSection.WriteString("    mov rdx, 58\n")
			c.textSection.WriteString("    syscall\n")

			c.textSection.WriteString("    mov rax, [rsp + 8]\n") // peek body_size
			c.textSection.WriteString("    mov rdi, net_scratch_buf\n")
			c.textSection.WriteString("    call num_to_str\n")

			c.textSection.WriteString("    mov rdi, [rsp]\n")
			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString("    syscall\n")

			headerSuffixLabel := fmt.Sprintf("http_suffix_%d", c.labelCounter)
			c.labelCounter++
			suffix := "\\r\\nConnection: close\\r\\n\\r\\n"
			c.dataSection.WriteString(fmt.Sprintf("    %s db `%s`, 0\n", headerSuffixLabel, suffix))

			c.textSection.WriteString("    mov rdi, [rsp]\n")
			c.textSection.WriteString("    mov rax, 1\n")
			c.textSection.WriteString(fmt.Sprintf("    mov rsi, %s\n", headerSuffixLabel))
			c.textSection.WriteString("    mov rdx, 23\n")
			c.textSection.WriteString("    syscall\n")

			c.textSection.WriteString("    pop rax\n")    // return client_fd
			c.textSection.WriteString("    add rsp, 8\n") // cleanup body_size
			return nil
		}

		// Multi-dispatch: Actor IPC or Function Call
		var argCount = len(n.Arguments)
		callId := c.labelCounter
		c.labelCounter++

		// Evaluate and push arguments (up to 6, System V ABI registers)
		var err error
		for i := 0; i < argCount && i < 6; i++ {
			err = c.Compile(n.Arguments[i])
			if err != nil {
				return err
			}
			c.textSection.WriteString("    push rax\n")
		}

		// Compile the function/actor expression
		err = c.Compile(n.Function)
		if err != nil {
			return err
		}

		// Heuristic: actor write fds are small (< 4096), function pointers are large
		c.textSection.WriteString("    cmp rax, 4096\n")
		c.textSection.WriteString(fmt.Sprintf("    ja .is_fn_call_%d\n", callId))

		c.textSection.WriteString(fmt.Sprintf(".is_actor_call_%d:\n", callId))
		if argCount == 0 {
			c.textSection.WriteString("    push 0\n")
		}
		c.textSection.WriteString("    mov rdi, rax\n")
		c.textSection.WriteString("    mov rax, 1\n")
		c.textSection.WriteString("    mov rsi, rsp\n")
		c.textSection.WriteString("    mov rdx, 8\n")
		c.textSection.WriteString("    syscall\n")
		c.textSection.WriteString("    pop rax\n")
		c.textSection.WriteString(fmt.Sprintf("    jmp .call_done_%d\n", callId))

		c.textSection.WriteString(fmt.Sprintf(".is_fn_call_%d:\n", callId))
		// Pop args into System V ABI registers (reversed since stack is LIFO)
		sysVRegs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		for i := argCount - 1; i >= 0 && i < 6; i-- {
			c.textSection.WriteString(fmt.Sprintf("    pop %s\n", sysVRegs[i]))
		}
		c.textSection.WriteString("    call rax\n")

		c.textSection.WriteString(fmt.Sprintf(".call_done_%d:\n", callId))

	case *ast.ActorLiteral:
		if c.target == TargetWindows {
			return fmt.Errorf("actors are not available on the windows backend yet")
		}
		actorId := c.labelCounter
		c.labelCounter++

		pipeLabel := fmt.Sprintf("pipe_fd_%d", actorId)
		bufLabel := fmt.Sprintf("actor_buf_%d", actorId)

		c.bssSection.WriteString(fmt.Sprintf("    %s resd 2\n", pipeLabel))
		c.bssSection.WriteString(fmt.Sprintf("    %s resq 1\n", bufLabel))

		c.textSection.WriteString(fmt.Sprintf("    ; --- spawning actor %d ---\n", actorId))
		c.textSection.WriteString("    mov rax, 22\n")
		c.textSection.WriteString(fmt.Sprintf("    mov rdi, %s\n", pipeLabel))
		c.textSection.WriteString("    syscall\n")

		c.textSection.WriteString("    mov rax, 57\n")
		c.textSection.WriteString("    syscall\n")
		c.textSection.WriteString("    test rax, rax\n")
		c.textSection.WriteString(fmt.Sprintf("    jz .child_%d\n", actorId))

		// PARENT: return write end of pipe
		c.textSection.WriteString(fmt.Sprintf(".parent_%d:\n", actorId))
		c.textSection.WriteString("    mov rax, 0\n")
		c.textSection.WriteString(fmt.Sprintf("    mov eax, dword [%s + 4]\n", pipeLabel))
		c.textSection.WriteString(fmt.Sprintf("    jmp .end_actor_%d\n", actorId))

		// CHILD: event loop
		c.textSection.WriteString(fmt.Sprintf(".child_%d:\n", actorId))
		c.textSection.WriteString(fmt.Sprintf(".actor_loop_%d:\n", actorId))
		c.textSection.WriteString("    mov rax, 0\n")
		c.textSection.WriteString(fmt.Sprintf("    mov edi, dword [%s]\n", pipeLabel))
		c.textSection.WriteString(fmt.Sprintf("    mov rsi, %s\n", bufLabel))
		c.textSection.WriteString("    mov rdx, 8\n")
		c.textSection.WriteString("    syscall\n")

		// Allocate actor argument as local
		c.currentOffset += 8
		c.locals[n.Argument.Value] = c.currentOffset
		c.textSection.WriteString(fmt.Sprintf("    mov rax, qword [%s]\n", bufLabel))
		c.textSection.WriteString(fmt.Sprintf("    mov [rbp - %d], rax\n", c.currentOffset))

		err := c.Compile(n.Body)
		if err != nil {
			return err
		}

		c.textSection.WriteString(fmt.Sprintf("    jmp .actor_loop_%d\n", actorId))
		c.textSection.WriteString(fmt.Sprintf(".end_actor_%d:\n", actorId))

	case *ast.StringLiteral:
		strLabel := fmt.Sprintf("str_%d", c.labelCounter)
		c.labelCounter++

		c.dataSection.WriteString(fmt.Sprintf("    %s db `%s`, 0\n", strLabel, n.Value))
		c.textSection.WriteString(fmt.Sprintf("    mov rax, %s\n", strLabel))

	case *ast.LetStatement:
		err := c.Compile(n.Value)
		if err != nil {
			return err
		}

		// CHECK: Are we at the top level?
		if c.currentFunctionEnd == "" {
			// --- GLOBAL VARIABLE ---
			label := fmt.Sprintf("glob_%s", n.Name.Value)
			// Reserve 8 bytes in the .bss section
			c.bssSection.WriteString(fmt.Sprintf("    %s resq 1\n", label))
			// Save the label in our global map
			c.globals[n.Name.Value] = label
			// Emit ASM to store the result (rax) into that memory address
			c.textSection.WriteString(fmt.Sprintf("    mov [%s], rax ; global let\n", label))
		} else {
			// --- LOCAL VARIABLE ---
			c.currentOffset += 8
			c.locals[n.Name.Value] = c.currentOffset
			c.textSection.WriteString(fmt.Sprintf("    mov [rbp - %d], rax ; local let\n", c.currentOffset))
		}
	case *ast.PrintStatement:
		err := c.Compile(n.Value)
		if err != nil {
			return err
		}

		c.textSection.WriteString("    ; print\n")
		c.textSection.WriteString("    call print_int\n")

	case *ast.ReturnStatement:
		if c.currentFunctionEnd == "" {
			return fmt.Errorf("return statement outside of function")
		}
		err := c.Compile(n.ReturnValue)
		if err != nil {
			return err
		}
		c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", c.currentFunctionEnd))

	case *ast.FunctionLiteral:
		if c.target == TargetWindows {
			return c.compileWindowsFunctionLiteral(n)
		}
		fnId := c.labelCounter
		c.labelCounter++
		fnLabel := fmt.Sprintf("fn_%d", fnId)
		endLabel := fmt.Sprintf("real_end_fn_%d", fnId)
		skipLabel := fmt.Sprintf("skip_fn_%d", fnId)

		// Save outer scope so functions don't pollute the global locals map
		oldLocals := make(map[string]int, len(c.locals))
		for k, v := range c.locals {
			oldLocals[k] = v
		}
		oldOffset := c.currentOffset
		c.currentOffset = 0 // each function gets its own fresh frame

		oldEnd := c.currentFunctionEnd
		c.currentFunctionEnd = endLabel

		// Skip over the function body in main flow
		c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", skipLabel))
		c.textSection.WriteString(fmt.Sprintf("%s:\n", fnLabel))
		c.textSection.WriteString("    push rbp\n")
		c.textSection.WriteString("    mov rbp, rsp\n")
		c.textSection.WriteString("    sub rsp, 512\n") // reserve frame for function locals

		// System V ABI: first 6 args in rdi, rsi, rdx, rcx, r8, r9
		sysVRegs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		for i, param := range n.Parameters {
			if i >= 6 {
				break
			}
			c.currentOffset += 8
			c.locals[param.Value] = c.currentOffset
			c.textSection.WriteString(fmt.Sprintf("    mov [rbp - %d], %s\n", c.currentOffset, sysVRegs[i]))
		}

		err := c.Compile(n.Body)
		if err != nil {
			return err
		}

		c.textSection.WriteString(fmt.Sprintf("%s:\n", endLabel))
		c.textSection.WriteString("    leave\n")
		c.textSection.WriteString("    ret\n")
		c.textSection.WriteString(fmt.Sprintf("%s:\n", skipLabel))

		// Restore outer scope
		c.locals = oldLocals
		c.currentOffset = oldOffset
		c.currentFunctionEnd = oldEnd

		c.textSection.WriteString(fmt.Sprintf("    mov rax, %s\n", fnLabel))
	case *ast.Identifier:
		// 1. Check if it's a global first (Permanent memory)
		if label, isGlobal := c.globals[n.Value]; isGlobal {
			c.textSection.WriteString(fmt.Sprintf("    mov rax, [%s]\n", label))
			return nil
		}

		// 2. Check if it's a local (Stack memory)
		offset, ok := c.locals[n.Value]
		if ok {
			c.textSection.WriteString(fmt.Sprintf("    mov rax, [rbp - %d]\n", offset))
			return nil
		}

		// 3. Whitelist for built-ins (The "is_alpha" fix)
		builtIns := []string{"disk_avail", "disk_total", "is_alpha", "is_digit", "get_byte", "set_byte", "get_uint16", "str_cmp", "str_to_int", "mem_alloc", "mem_free", "sys_argc", "sys_argv", "sys_exec", "sys_fork", "sys_wait", "print_str", "array", "mem_get", "mem_set", "str_len", "exit", "sys_exit"}
		for _, bi := range builtIns {
			if n.Value == bi {
				if c.target == TargetWindows {
					if _, ok := windowsBuiltinNames[n.Value]; ok {
						return nil
					}
					if _, ok := windowsUnsupportedBuiltins[n.Value]; ok {
						return nil
					}
				}
				return nil
			}
		}

		// 4. Built-in buffers
		if n.Value == "file_io_buf" || n.Value == "net_request_buf" || n.Value == "net_scratch_buf" {
			c.textSection.WriteString(fmt.Sprintf("    mov rax, %s\n", n.Value))
			return nil
		}

		return fmt.Errorf("undefined variable: %s", n.Value)
	case *ast.IndexExpression:
		err := c.Compile(n.Left)
		if err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n") // save base ptr

		// Check for .len sentinel (index == -1)
		if il, ok := n.Index.(*ast.IntegerLiteral); ok && il.Value == -1 {
			c.textSection.WriteString("    pop rax\n")
			c.textSection.WriteString("    mov rax, [rax]\n") // read length header
			return nil
		}

		err = c.Compile(n.Index)
		if err != nil {
			return err
		}
		// ptr + 8 + index*8  (skip length header)
		c.textSection.WriteString("    shl rax, 3\n") // index * 8
		c.textSection.WriteString("    add rax, 8\n") // skip length header
		c.textSection.WriteString("    pop rbx\n")    // base
		c.textSection.WriteString("    add rbx, rax\n")
		c.textSection.WriteString("    mov rax, [rbx]\n")
	case *ast.IntegerLiteral:
		c.textSection.WriteString(fmt.Sprintf("    mov rax, %d\n", n.Value))

	case *ast.PrefixExpression:
		err := c.Compile(n.Right)
		if err != nil {
			return err
		}
		if n.Operator == "-" {
			c.textSection.WriteString("    neg rax\n")
		} else if n.Operator == "!" {
			c.textSection.WriteString("    test rax, rax\n")
			c.textSection.WriteString("    setz al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		}

	case *ast.InfixExpression:
		err := c.Compile(n.Left)
		if err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")

		err = c.Compile(n.Right)
		if err != nil {
			return err
		}
		c.textSection.WriteString("    mov rbx, rax\n")
		c.textSection.WriteString("    pop rax\n")

		switch n.Operator {
		case "+":
			c.textSection.WriteString("    add rax, rbx\n")
		case "-":
			c.textSection.WriteString("    sub rax, rbx\n")
		case "*":
			c.textSection.WriteString("    imul rax, rbx\n")
		case "/":
			c.textSection.WriteString("    cqo\n")
			c.textSection.WriteString("    idiv rbx\n")
		case "==":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    sete al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case "!=":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    setne al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case "<":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    setl al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case ">":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    setg al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case "<=":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    setle al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case "&&":
			// Logical AND: result is 1 only if both are non-zero
			c.textSection.WriteString("    test rax, rax\n")
			c.textSection.WriteString("    setnz al\n")
			c.textSection.WriteString("    movzx rax, al\n")
			c.textSection.WriteString("    test rbx, rbx\n")
			c.textSection.WriteString("    setnz bl\n")
			c.textSection.WriteString("    movzx rbx, bl\n")
			c.textSection.WriteString("    and rax, rbx\n")
		case "||":
			// Logical OR: result is 1 if either is non-zero
			c.textSection.WriteString("    or rax, rbx\n")
			c.textSection.WriteString("    setnz al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		case ">=":
			c.textSection.WriteString("    cmp rax, rbx\n")
			c.textSection.WriteString("    setge al\n")
			c.textSection.WriteString("    movzx rax, al\n")
		default:
			return fmt.Errorf("unknown operator %s", n.Operator)
		}
	}

	return nil
}

func (c *Compiler) Assemble() string {
	if c.target == TargetWindows {
		return c.assembleWindows()
	}
	var final bytes.Buffer

	final.WriteString("global _start\n\n")

	final.WriteString("section .data\n")
	final.WriteString("    newline db 10\n")
	final.WriteString("    bind_any_addr db `0.0.0.0`, 0\n")
	final.WriteString("    loopback_addr db `127.0.0.1`, 0\n")
	final.WriteString("    localhost_addr db `localhost`, 0\n")
	final.WriteString(c.dataSection.String())
	final.WriteString("\n")

	final.WriteString("section .bss\n")
	final.WriteString("    sys_argc_val resq 1\n") // Store argc here
	final.WriteString("    sys_argv_ptr resq 1\n") // Store argv pointer here
	final.WriteString("    print_buf resb 32\n")
	final.WriteString("    file_io_buf resb 65536\n")
	final.WriteString("    net_scratch_buf resb 128\n")
	final.WriteString("    errno resq 1\n")
	final.WriteString("    net_request_buf resb 4096\n")
	final.WriteString("    statfs_buf resb 128\n")
	final.WriteString("    sigint_actor_pipe resq 1\n")
	final.WriteString(c.bssSection.String())
	final.WriteString("\n")

	final.WriteString("section .text\n")

final.WriteString(`print_int:
    mov r8, print_buf
    add r8, 31
    mov byte [r8], 10
    mov r9, 10
    mov r10, 0
    xor r11, r11
    test rax, rax
    jns .loop
    neg rax
    mov r11, 1
.loop:
    xor rdx, rdx
    div r9
    add dl, '0'
    dec r8
    mov [r8], dl
    inc r10
    test rax, rax
    jnz .loop
    test r11, r11
    jz .emit
    dec r8
    mov byte [r8], '-'
    inc r10
.emit:
    mov rax, 1
    mov rdi, 1
    mov rsi, r8
    mov rdx, r10
    inc rdx
    syscall
    ret

str_start_with:
.next:
    mov al, [rsi]
    test al, al
    jz .match
    cmp al, [rdi]
    jne .no_match
    inc rsi
    inc rdi
    jmp .next
.match:
    mov rax, 1
    ret
.no_match:
    mov rax, 0
    ret

; print_str: Expects string pointer in rax
print_str:
    push rbp
    mov rbp, rsp
    mov rsi, rax
    mov rdx, 0
.loop:
    cmp byte [rsi + rdx], 0
    je .done
    inc rdx
    jmp .loop
.done:
    mov rdi, 1
    mov rax, 1
    syscall
    mov rax, 1
    mov rdi, 1
    mov rsi, newline
    mov rdx, 1
    syscall
    mov rsp, rbp
    pop rbp
    ret

; num_to_str: rax = number, rdi = buffer
; Returns: rdx = length, rsi = pointer (for sys_write)
num_to_str:
    push rbp
    mov rbp, rsp
    add rdi, 31
    mov byte [rdi], 0
    mov r9, 10
    mov r10, 0
    xor r11, r11
    test rax, rax
    jns .num_loop
    neg rax
    mov r11, 1
.num_loop:
    xor rdx, rdx
    div r9
    add dl, '0'
    dec rdi
    mov [rdi], dl
    inc r10
    test rax, rax
    jnz .num_loop
    test r11, r11
    jz .num_done
    dec rdi
    mov byte [rdi], '-'
    inc r10
.num_done:
    mov rdx, r10
    mov rsi, rdi
    mov rsp, rbp
    pop rbp
    ret

sig_restorer_stub:
    mov rax, 15
    syscall

sigint_stub:
    push rax
    push rdi
    push rsi
    push rdx
    mov rdi, [sigint_actor_pipe]
    test rdi, rdi
    jz .no_actor
    push 999
    mov rsi, rsp
    mov rdx, 8
    mov rax, 1
    syscall
    pop rax
.no_actor:
    pop rdx
    pop rsi
    pop rdi
    pop rax
    ret

; check_rax: if syscall result < 0, stores abs(rax) in errno and returns -1
check_rax:
    test rax, rax
    jns .ok
    neg rax
    mov [errno], rax
    mov rax, -1
.ok:
    ret

; parse_bind_host
; rdi = string pointer
; returns eax = IPv4 address in network byte order
; supports "0.0.0.0", "127.0.0.1", and "localhost"
parse_bind_host:
    mov rsi, bind_any_addr
    call asm_str_cmp
    test rax, rax
    jnz .any
    mov rsi, localhost_addr
    call asm_str_cmp
    test rax, rax
    jnz .loopback
    mov rsi, loopback_addr
    call asm_str_cmp
    test rax, rax
    jnz .loopback
.any:
    xor eax, eax
    ret
.loopback:
    mov eax, 0x0100007F
    ret

; find_path_in_request
; rdi = raw request buffer, rsi = path string to match
; Returns rax = 1 (match) or 0 (no match)
find_path_in_request:
    push rbx
.find_slash:
    cmp byte [rdi], '/'
    je .check_match
    cmp byte [rdi], 0
    je .no_match
    inc rdi
    jmp .find_slash
.check_match:
    mov rdx, rdi
    mov rcx, rsi
.loop:
    mov al, [rcx]
    test al, al
    jz .match
    mov bl, [rdx]
    cmp al, bl
    jne .next_try
    inc rcx
    inc rdx
    jmp .loop
.next_try:
    inc rdi
    jmp .find_slash
.match:
    pop rbx
    mov rax, 1
    ret
.no_match:
    pop rbx
    mov rax, 0
    ret

; copy_request_path_tail
; rdi = raw request buffer, rsi = prefix, rdx = destination buffer
; Returns rax = copied length, or -1 if prefix does not match request path
copy_request_path_tail:
    push rbx
    push r12
    mov r12, rdx
.seek_path:
    cmp byte [rdi], '/'
    je .compare
    cmp byte [rdi], 0
    je .no_match
    inc rdi
    jmp .seek_path
.compare:
    mov r8, rdi
    mov rcx, rsi
.prefix_loop:
    mov al, [rcx]
    test al, al
    jz .copy_tail
    mov bl, [r8]
    cmp al, bl
    jne .no_match
    inc rcx
    inc r8
    jmp .prefix_loop
.copy_tail:
    xor rax, rax
.copy_loop:
    mov bl, [r8]
    cmp bl, ' '
    je .done
    cmp bl, '?'
    je .done
    cmp bl, '#'
    je .done
    cmp bl, 13
    je .done
    mov [r12 + rax], bl
    inc r8
    inc rax
    jmp .copy_loop
.done:
    mov byte [r12 + rax], 0
    pop r12
    pop rbx
    ret
.no_match:
    pop r12
    pop rbx
    mov rax, -1
    ret

; request_path_eq
; rdi = raw request buffer, rsi = expected path
; Returns rax = 1 only when the request path exactly matches
request_path_eq:
    push rbx
.seek_path:
    cmp byte [rdi], '/'
    je .compare
    cmp byte [rdi], 0
    je .no_match
    inc rdi
    jmp .seek_path
.compare:
    mov rdx, rdi
    mov rcx, rsi
.compare_loop:
    mov al, [rcx]
    test al, al
    jz .check_boundary
    mov bl, [rdx]
    cmp al, bl
    jne .no_match
    inc rcx
    inc rdx
    jmp .compare_loop
.check_boundary:
    mov al, [rdx]
    cmp al, ' '
    je .match
    cmp al, '?'
    je .match
    cmp al, '#'
    je .match
    cmp al, 13
    je .match
    jmp .no_match
.match:
    pop rbx
    mov rax, 1
    ret
.no_match:
    pop rbx
    mov rax, 0
    ret
`)

	final.WriteString("_start:\n")
	// 1. Grab argc and argv from the stack
	final.WriteString("    mov rax, [rsp]\n") // argc is at the top of the stack
	final.WriteString("    mov [sys_argc_val], rax\n")
	final.WriteString("    lea rax, [rsp + 8]\n") // argv array starts 8 bytes down
	final.WriteString("    mov [sys_argv_ptr], rax\n")

	final.WriteString("    push rbp\n")
	final.WriteString("    mov rbp, rsp\n")
	frameSize := c.currentOffset
	if frameSize < 512 {
		frameSize = 512
	}
	frameSize = (frameSize + 15) &^ 15
	final.WriteString(fmt.Sprintf("    sub rsp, %d\n", frameSize))

	final.WriteString(c.textSection.String())

	final.WriteString("    mov rsp, rbp\n")
	final.WriteString("    pop rbp\n")
	final.WriteString("    ; syscall exit(0)\n")
	final.WriteString("    mov rax, 60\n")
	final.WriteString("    mov rdi, 0\n")
	final.WriteString("    syscall\n")
	final.WriteString(`
; write_str_fd: rdi = fd, rax = string pointer
write_str_fd:
    push rbp
    mov rbp, rsp
    mov rsi, rax
    mov rdx, 0
.wstr_loop:
    cmp byte [rsi + rdx], 0
    je .wstr_done
    inc rdx
    jmp .wstr_loop
.wstr_done:
    mov rax, 1      ; sys_write
    syscall
    mov rsp, rbp
    pop rbp
    ret

	asm_str_cmp:
.loop:
    mov al, [rdi]
    mov bl, [rsi]
    cmp al, bl
    jne .not_equal
    test al, al      ; If we hit null and they matched, they are equal
    jz .equal
    inc rdi
    inc rsi
    jmp .loop
.not_equal:
    xor rax, rax
    ret
.equal:
    mov rax, 1
    ret

	asm_is_digit:
    cmp rax, '0'
    jl .no
    cmp rax, '9'
    jg .no
    mov rax, 1
    ret
.no: xor rax, rax
    ret

asm_is_alpha:
    ; Checks A-Z, a-z, and _
    cmp rax, '_'
    je .yes
    cmp rax, 'A'
    jl .no
    cmp rax, 'Z'
    jle .yes
    cmp rax, 'a'
    jl .no
    cmp rax, 'z'
    jg .no
.yes: mov rax, 1
    ret
.no: xor rax, rax
    ret

	asm_str_to_int:
    xor rax, rax
    xor rcx, rcx
.loop:
    movzx rdx, byte [rdi + rcx]
    cmp rdx, '0'
    jl .done
    cmp rdx, '9'
    jg .done
    sub rdx, '0'
    imul rax, 10
    add rax, rdx
    inc rcx
    jmp .loop
.done:
    ret

	; int_to_buf(number, buffer_ptr)
; Converts number to string and writes it into buffer_ptr
; Returns the length of the string in rax
int_to_buf:
    push rbp
    mov rbp, rsp
    mov rax, rdi      ; The number
    mov r8, rsi       ; The buffer pointer
    mov r9, 10        ; Divisor
    mov r10, 0        ; Digit counter
    xor r11, r11
    test rax, rax
    jns .loop
    neg rax
    mov r11, 1
    
    ; We need to find the end of the number first to write backwards
    ; or use a temporary stack-based approach. 
    ; (Simplified version):
.loop:
    xor rdx, rdx
    div r9
    add dl, '0'
    push rdx          ; Save digit
    inc r10
    test rax, rax
    jnz .loop
    
    mov rax, r10      ; Return the length
.pop_loop:
    pop rdx
    mov [r8], dl
    inc r8
    dec r10
    jnz .pop_loop
    test r11, r11
    jz .done
    mov byte [r8], '-'
    inc r8
    inc rax
.done:
    mov byte [r8], 0  ; Null terminate
    pop rbp
    ret
`)

	return final.String()
}
