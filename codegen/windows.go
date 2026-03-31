package codegen

import (
	"bytes"
	"fmt"
	"t2f/ast"
)

var windowsBuiltinNames = map[string]struct{}{
	"array":      {},
	"exit":       {},
	"get_byte":   {},
	"get_uint16": {},
	"is_alpha":   {},
	"is_digit":   {},
	"int_to_buf": {},
	"mem_alloc":  {},
	"mem_free":   {},
	"mem_get":    {},
	"mem_set":    {},
	"print_str":  {},
	"set_byte":   {},
	"str_cmp":    {},
	"str_len":    {},
	"str_to_int": {},
	"sys_argc":   {},
	"sys_argv":   {},
	"sys_exit":   {},
}

var windowsUnsupportedBuiltins = map[string]struct{}{
	"disk_avail":          {},
	"disk_total":          {},
	"fs_read_to":          {},
	"net_copy_path_tail":  {},
	"net_error":           {},
	"net_match_path":      {},
	"net_on_interrupt":    {},
	"net_path_eq":         {},
	"net_reuse_addr":      {},
	"net_send_int":        {},
	"net_send_status_ok":  {},
	"net_write":           {},
	"sys_exec":            {},
	"sys_fork":            {},
	"sys_getdents":        {},
	"sys_mkdir":           {},
	"sys_unlink":          {},
	"sys_wait":            {},
	"write_str":           {},
}

func (c *Compiler) compileWindowsArrayLiteral(n *ast.ArrayLiteral) error {
	count := len(n.Elements)
	size := count*8 + 8

	c.textSection.WriteString("    mov rcx, [process_heap]\n")
	c.textSection.WriteString("    mov rdx, 8\n")
	c.textSection.WriteString(fmt.Sprintf("    mov r8, %d\n", size))
	c.textSection.WriteString("    call HeapAlloc\n")
	c.textSection.WriteString(fmt.Sprintf("    mov qword [rax], %d\n", count))
	c.textSection.WriteString("    push rax\n")

	for i, elem := range n.Elements {
		if err := c.Compile(elem); err != nil {
			return err
		}
		offset := 8 + i*8
		c.textSection.WriteString("    mov rcx, [rsp]\n")
		c.textSection.WriteString(fmt.Sprintf("    mov [rcx + %d], rax\n", offset))
	}

	c.textSection.WriteString("    pop rax\n")
	return nil
}

func (c *Compiler) compileWindowsCallExpression(n *ast.CallExpression) error {
	var funcName string
	if ident, ok := n.Function.(*ast.Identifier); ok {
		funcName = ident.Value
	}

	if _, unsupported := windowsUnsupportedBuiltins[funcName]; unsupported {
		return fmt.Errorf("%s is not available on the windows backend yet", funcName)
	}

	switch funcName {
	case "sys_argc":
		c.textSection.WriteString("    mov rax, [sys_argc_val]\n")
		return nil
	case "sys_argv":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov rbx, [sys_argv_ptr]\n")
		c.textSection.WriteString("    shl rax, 3\n")
		c.textSection.WriteString("    add rbx, rax\n")
		c.textSection.WriteString("    mov rax, [rbx]\n")
		return nil
	case "exit", "sys_exit":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov ecx, eax\n")
		c.textSection.WriteString("    call ExitProcess\n")
		return nil
	case "set_byte":
		if err := c.Compile(n.Arguments[2]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    pop rcx\n")
		c.textSection.WriteString("    add rax, rbx\n")
		c.textSection.WriteString("    mov [rax], cl\n")
		return nil
	case "get_uint16":
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    add rax, rbx\n")
		c.textSection.WriteString("    movzx rax, word [rax]\n")
		return nil
	case "get_byte":
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    add rax, rbx\n")
		c.textSection.WriteString("    movzx rax, byte [rax]\n")
		return nil
	case "mem_set":
		if err := c.Compile(n.Arguments[2]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    pop rcx\n")
		c.textSection.WriteString("    shl rbx, 3\n")
		c.textSection.WriteString("    add rax, rbx\n")
		c.textSection.WriteString("    mov [rax], rcx\n")
		return nil
	case "mem_get":
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    shl rbx, 3\n")
		c.textSection.WriteString("    add rax, rbx\n")
		c.textSection.WriteString("    mov rax, [rax]\n")
		return nil
	case "mem_alloc":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov r8, rax\n")
		c.textSection.WriteString("    mov rcx, [process_heap]\n")
		c.textSection.WriteString("    mov rdx, 8\n")
		c.textSection.WriteString("    call HeapAlloc\n")
		return nil
	case "mem_free":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov r8, rax\n")
		c.textSection.WriteString("    mov rcx, [process_heap]\n")
		c.textSection.WriteString("    xor edx, edx\n")
		c.textSection.WriteString("    call HeapFree\n")
		return nil
	case "array":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		c.textSection.WriteString("    lea r8, [rax*8+8]\n")
		c.textSection.WriteString("    mov rcx, [process_heap]\n")
		c.textSection.WriteString("    mov rdx, 8\n")
		c.textSection.WriteString("    call HeapAlloc\n")
		c.textSection.WriteString("    pop rbx\n")
		c.textSection.WriteString("    mov [rax], rbx\n")
		return nil
	case "str_to_int":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov rcx, rax\n")
		c.textSection.WriteString("    call asm_str_to_int\n")
		return nil
	case "int_to_buf":
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov rcx, rax\n")
		c.textSection.WriteString("    pop rdx\n")
		c.textSection.WriteString("    call int_to_buf\n")
		return nil
	case "str_len":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    mov rcx, rax\n")
		c.textSection.WriteString("    xor rax, rax\n")
		strLenLoop := fmt.Sprintf(".win_str_len_loop_%d", c.labelCounter)
		strLenDone := fmt.Sprintf(".win_str_len_done_%d", c.labelCounter)
		c.labelCounter++
		c.textSection.WriteString(fmt.Sprintf("%s:\n", strLenLoop))
		c.textSection.WriteString("    cmp byte [rcx + rax], 0\n")
		c.textSection.WriteString(fmt.Sprintf("    je %s\n", strLenDone))
		c.textSection.WriteString("    inc rax\n")
		c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", strLenLoop))
		c.textSection.WriteString(fmt.Sprintf("%s:\n", strLenDone))
		return nil
	case "str_cmp":
		if err := c.Compile(n.Arguments[1]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    pop rdx\n")
		c.textSection.WriteString("    mov rcx, rax\n")
		c.textSection.WriteString("    call asm_str_cmp\n")
		return nil
	case "print_str":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    call print_str\n")
		return nil
	case "is_digit":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    call asm_is_digit\n")
		return nil
	case "is_alpha":
		if err := c.Compile(n.Arguments[0]); err != nil {
			return err
		}
		c.textSection.WriteString("    call asm_is_alpha\n")
		return nil
	}

	argCount := len(n.Arguments)
	if argCount > 4 {
		return fmt.Errorf("windows backend currently supports up to 4 function arguments, got %d", argCount)
	}

	callID := c.labelCounter
	c.labelCounter++

	for i := 0; i < argCount; i++ {
		if err := c.Compile(n.Arguments[i]); err != nil {
			return err
		}
		c.textSection.WriteString("    push rax\n")
	}

	if err := c.Compile(n.Function); err != nil {
		return err
	}

	winRegs := []string{"rcx", "rdx", "r8", "r9"}
	for i := argCount - 1; i >= 0; i-- {
		c.textSection.WriteString(fmt.Sprintf("    pop %s\n", winRegs[i]))
	}
	c.textSection.WriteString(fmt.Sprintf("    ; win64 call %d\n", callID))
	c.textSection.WriteString("    call rax\n")
	return nil
}

func (c *Compiler) compileWindowsFunctionLiteral(n *ast.FunctionLiteral) error {
	if len(n.Parameters) > 4 {
		return fmt.Errorf("windows backend currently supports up to 4 function parameters, got %d", len(n.Parameters))
	}

	fnID := c.labelCounter
	c.labelCounter++
	fnLabel := fmt.Sprintf("fn_%d", fnID)
	endLabel := fmt.Sprintf("real_end_fn_%d", fnID)
	skipLabel := fmt.Sprintf("skip_fn_%d", fnID)

	oldLocals := make(map[string]int, len(c.locals))
	for k, v := range c.locals {
		oldLocals[k] = v
	}
	oldOffset := c.currentOffset
	c.currentOffset = 0

	oldEnd := c.currentFunctionEnd
	c.currentFunctionEnd = endLabel

	c.textSection.WriteString(fmt.Sprintf("    jmp %s\n", skipLabel))
	c.textSection.WriteString(fmt.Sprintf("%s:\n", fnLabel))
	c.textSection.WriteString("    push rbp\n")
	c.textSection.WriteString("    mov rbp, rsp\n")
	c.textSection.WriteString("    sub rsp, 544\n")

	winRegs := []string{"rcx", "rdx", "r8", "r9"}
	for i, param := range n.Parameters {
		c.currentOffset += 8
		c.locals[param.Value] = c.currentOffset
		c.textSection.WriteString(fmt.Sprintf("    mov [rbp - %d], %s\n", c.currentOffset, winRegs[i]))
	}

	if err := c.Compile(n.Body); err != nil {
		return err
	}

	c.textSection.WriteString(fmt.Sprintf("%s:\n", endLabel))
	c.textSection.WriteString("    mov rsp, rbp\n")
	c.textSection.WriteString("    pop rbp\n")
	c.textSection.WriteString("    ret\n")
	c.textSection.WriteString(fmt.Sprintf("%s:\n", skipLabel))

	c.locals = oldLocals
	c.currentOffset = oldOffset
	c.currentFunctionEnd = oldEnd

	c.textSection.WriteString(fmt.Sprintf("    mov rax, %s\n", fnLabel))
	return nil
}

func (c *Compiler) assembleWindows() string {
	var final bytes.Buffer

	final.WriteString("default rel\n")
	final.WriteString("extern ExitProcess\n")
	final.WriteString("extern GetStdHandle\n")
	final.WriteString("extern WriteFile\n")
	final.WriteString("extern GetProcessHeap\n")
	final.WriteString("extern HeapAlloc\n")
	final.WriteString("extern HeapFree\n")
	final.WriteString("global main\n\n")

	final.WriteString("section .data\n")
	final.WriteString("    newline db 10\n")
	final.WriteString(c.dataSection.String())
	final.WriteString("\n")

	final.WriteString("section .bss\n")
	final.WriteString("    sys_argc_val resq 1\n")
	final.WriteString("    sys_argv_ptr resq 1\n")
	final.WriteString("    stdout_handle resq 1\n")
	final.WriteString("    process_heap resq 1\n")
	final.WriteString("    bytes_written resq 1\n")
	final.WriteString("    print_buf resb 32\n")
	final.WriteString("    file_io_buf resb 65536\n")
	final.WriteString("    net_scratch_buf resb 128\n")
	final.WriteString("    errno resq 1\n")
	final.WriteString("    net_request_buf resb 4096\n")
	final.WriteString("    sigint_actor_pipe resq 1\n")
	final.WriteString(c.bssSection.String())
	final.WriteString("\n")

	final.WriteString("section .text\n")
	final.WriteString(`write_console:
    push rbp
    mov rbp, rsp
    sub rsp, 48
    mov rcx, [stdout_handle]
    mov r8, rdx
    mov rdx, rsi
    lea r9, [bytes_written]
    mov qword [rsp + 32], 0
    call WriteFile
    mov rsp, rbp
    pop rbp
    ret

print_int:
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
    mov rsi, r8
    mov rdx, r10
    inc rdx
    call write_console
    ret

print_str:
    push rbp
    mov rbp, rsp
    mov rsi, rax
    xor rdx, rdx
.loop:
    cmp byte [rsi + rdx], 0
    je .done
    inc rdx
    jmp .loop
.done:
    call write_console
    lea rsi, [newline]
    mov rdx, 1
    call write_console
    mov rsp, rbp
    pop rbp
    ret

asm_str_cmp:
.loop:
    mov al, [rcx]
    mov bl, [rdx]
    cmp al, bl
    jne .not_equal
    test al, al
    jz .equal
    inc rcx
    inc rdx
    jmp .loop
.not_equal:
    xor rax, rax
    ret
.equal:
    mov rax, 1
    ret

asm_str_to_int:
    xor rax, rax
    xor r8, r8
.loop:
    movzx rdx, byte [rcx + r8]
    cmp rdx, '0'
    jl .done
    cmp rdx, '9'
    jg .done
    sub rdx, '0'
    imul rax, 10
    add rax, rdx
    inc r8
    jmp .loop
.done:
    ret

int_to_buf:
    push rbp
    mov rbp, rsp
    mov rax, rcx
    mov r8, rdx
    mov r9, 10
    xor r10, r10
    xor r11, r11
    test rax, rax
    jns .loop
    neg rax
    mov r11, 1
.loop:
    xor rdx, rdx
    div r9
    add dl, '0'
    push rdx
    inc r10
    test rax, rax
    jnz .loop
    mov rax, r10
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
    mov byte [r8], 0
    mov rsp, rbp
    pop rbp
    ret

asm_is_digit:
    cmp rax, '0'
    jl .no
    cmp rax, '9'
    jg .no
    mov rax, 1
    ret
.no:
    xor rax, rax
    ret

asm_is_alpha:
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
.yes:
    mov rax, 1
    ret
.no:
    xor rax, rax
    ret

main:
    mov [sys_argc_val], rcx
    mov [sys_argv_ptr], rdx
    push rbp
    mov rbp, rsp
    sub rsp, 544
    mov ecx, -11
    call GetStdHandle
    mov [stdout_handle], rax
    call GetProcessHeap
    mov [process_heap], rax
`)

	final.WriteString(c.textSection.String())

	final.WriteString(`    xor ecx, ecx
    call ExitProcess
`)

	return final.String()
}
