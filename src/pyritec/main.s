.section .text
.globl main
main:
    push rbp
    mov rbp, rsp
    push rbx
    sub rsp, 24

    cmp edi, 3
    jl .usage

    mov rbx, rsi
    mov rax, QWORD PTR [rbx+16]
    mov QWORD PTR [rip+out_path], rax

    mov rdi, QWORD PTR [rbx+8]
    lea rsi, [rip+mode_r]
    call fopen
    test rax, rax
    je .open_error
    mov QWORD PTR [rip+in_file], rax

    lea rdi, [rip+temp_asm]
    lea rsi, [rip+mode_w]
    call fopen
    test rax, rax
    je .open_error
    mov QWORD PTR [rip+out_file], rax

    lea rdi, [rip+asm_header]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs
    lea rdi, [rip+asm_int_fmt]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs

    call emit_rodata_pass

    mov rdi, QWORD PTR [rip+in_file]
    call fclose

    mov rdi, QWORD PTR [rbx+8]
    lea rsi, [rip+mode_r]
    call fopen
    test rax, rax
    je .open_error
    mov QWORD PTR [rip+in_file], rax

    lea rdi, [rip+asm_text]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs

    mov QWORD PTR [rip+string_count], 0
    call emit_text_pass

    cmp QWORD PTR [rip+compile_error], 0
    jne .syntax_error
    cmp QWORD PTR [rip+saw_main], 0
    je .syntax_error

    cmp QWORD PTR [rip+emitted_return], 0
    jne .close_and_link
    lea rdi, [rip+asm_default_ret]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs

.close_and_link:
    mov rdi, QWORD PTR [rip+in_file]
    call fclose
    mov rdi, QWORD PTR [rip+out_file]
    call fclose

    lea rdi, [rip+cmd_buf]
    mov esi, 1024
    lea rdx, [rip+link_cmd_fmt]
    mov rcx, QWORD PTR [rip+out_path]
    xor eax, eax
    call snprintf

    lea rdi, [rip+cmd_buf]
    call system
    test eax, eax
    jne .link_error

    lea rdi, [rip+temp_asm]
    call remove

    xor eax, eax
    jmp .exit

.usage:
    lea rdi, [rip+usage_msg]
    mov rsi, QWORD PTR [rip+stderr]
    call fputs
    mov eax, 2
    jmp .exit

.open_error:
    lea rdi, [rip+open_msg]
    mov rsi, QWORD PTR [rip+stderr]
    call fputs
    mov eax, 1
    jmp .exit

.link_error:
    lea rdi, [rip+link_msg]
    mov rsi, QWORD PTR [rip+stderr]
    call fputs
    mov eax, 1
    jmp .exit

.syntax_error:
    lea rdi, [rip+syntax_msg]
    mov rsi, QWORD PTR [rip+stderr]
    call fputs
    mov eax, 1

.exit:
    add rsp, 24
    pop rbx
    pop rbp
    ret

emit_rodata_pass:
    push rbp
    mov rbp, rsp
    push rbx
    sub rsp, 8

.ro_loop:
    lea rdi, [rip+linebuf]
    mov esi, 512
    mov rdx, QWORD PTR [rip+in_file]
    call fgets
    test rax, rax
    je .ro_done

    lea rdi, [rip+linebuf]
    call skip_ws
    mov rbx, rax

    mov rdi, rbx
    lea rsi, [rip+kw_print]
    mov edx, 6
    call strncmp
    test eax, eax
    jne .ro_loop
    cmp BYTE PTR [rbx+6], '"'
    jne .ro_loop

    mov rdi, rbx
    call emit_string_constant
    jmp .ro_loop

.ro_done:
    add rsp, 8
    pop rbx
    pop rbp
    ret

emit_text_pass:
    push rbp
    mov rbp, rsp
    push rbx
    sub rsp, 8

.text_loop:
    lea rdi, [rip+linebuf]
    mov esi, 512
    mov rdx, QWORD PTR [rip+in_file]
    call fgets
    test rax, rax
    je .text_done

    lea rdi, [rip+linebuf]
    call skip_ws
    mov rbx, rax

    mov rdi, rbx
    lea rsi, [rip+kw_import]
    mov edx, 7
    call strncmp
    test eax, eax
    je .text_loop

    mov rdi, rbx
    lea rsi, [rip+kw_def_main]
    mov edx, 9
    call strncmp
    test eax, eax
    jne .check_print
    mov QWORD PTR [rip+saw_main], 1
    jmp .text_loop

.check_print:
    mov rdi, rbx
    lea rsi, [rip+kw_print]
    mov edx, 6
    call strncmp
    test eax, eax
    jne .check_return
    cmp BYTE PTR [rbx+6], '"'
    jne .print_var
    call emit_print_call
    jmp .text_loop

.print_var:
    mov rdi, rbx
    call emit_print_var
    jmp .text_loop

.check_return:
    mov rdi, rbx
    lea rsi, [rip+kw_return]
    mov edx, 7
    call strncmp
    test eax, eax
    jne .check_assignment
    mov rdi, rbx
    call emit_return
    mov QWORD PTR [rip+emitted_return], 1
    jmp .text_loop

.check_assignment:
    movzx eax, BYTE PTR [rbx]
    cmp al, 0
    je .text_loop
    cmp al, 10
    je .text_loop
    cmp al, '#'
    je .text_loop
    mov rdi, rbx
    call emit_assignment
    jmp .text_loop

.unsupported_line:
    movzx eax, BYTE PTR [rbx]
    cmp al, 0
    je .text_loop
    cmp al, 10
    je .text_loop
    cmp al, '#'
    je .text_loop
    mov QWORD PTR [rip+compile_error], 1
    jmp .text_loop

.text_done:
    add rsp, 8
    pop rbx
    pop rbp
    ret

emit_string_constant:
    push rbp
    mov rbp, rsp
    push rbx
    push r12

    mov rbx, rdi

    mov rdi, QWORD PTR [rip+out_file]
    lea rsi, [rip+str_label_fmt]
    mov rdx, QWORD PTR [rip+string_count]
    xor eax, eax
    call fprintf

    mov rdi, rbx
    mov esi, '"'
    call strchr
    test rax, rax
    je .str_done
    lea r12, [rax+1]

.str_copy:
    movzx eax, BYTE PTR [r12]
    cmp al, 0
    je .str_done
    cmp al, 10
    je .str_done
    cmp al, '"'
    je .str_done
    mov edi, eax
    mov rsi, QWORD PTR [rip+out_file]
    call fputc
    inc r12
    jmp .str_copy

.str_done:
    lea rdi, [rip+str_label_end]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs
    add QWORD PTR [rip+string_count], 1

    pop r12
    pop rbx
    pop rbp
    ret

emit_print_call:
    push rbp
    mov rbp, rsp
    mov rdi, QWORD PTR [rip+out_file]
    lea rsi, [rip+asm_print_fmt]
    mov rdx, QWORD PTR [rip+string_count]
    xor eax, eax
    call fprintf
    add QWORD PTR [rip+string_count], 1
    pop rbp
    ret

emit_print_var:
    push rbp
    mov rbp, rsp
    movzx eax, BYTE PTR [rdi+6]
    cmp BYTE PTR [rdi+7], ')'
    jne .bad_print_var
    cmp al, 'a'
    je .print_a
    cmp al, 'b'
    je .print_b
    cmp al, 'c'
    je .print_c
.bad_print_var:
    mov QWORD PTR [rip+compile_error], 1
    pop rbp
    ret
.print_a:
    lea rdi, [rip+asm_print_a]
    jmp .emit_print_var
.print_b:
    lea rdi, [rip+asm_print_b]
    jmp .emit_print_var
.print_c:
    lea rdi, [rip+asm_print_c]
.emit_print_var:
    mov rsi, QWORD PTR [rip+out_file]
    call fputs
    pop rbp
    ret

emit_assignment:
    push rbp
    mov rbp, rsp
    push rbx
    push r12
    push r13
    push r14

    mov rbx, rdi
    xor r13d, r13d

    mov rdi, rbx
    lea rsi, [rip+kw_set]
    mov edx, 4
    call strncmp
    test eax, eax
    jne .assign_var
    mov r13d, 1
    add rbx, 4
    mov rdi, rbx
    call skip_ws
    mov rbx, rax

.assign_var:
    movzx eax, BYTE PTR [rbx]
    cmp al, 'a'
    je .var_a
    cmp al, 'b'
    je .var_b
    cmp al, 'c'
    je .var_c
    jmp .bad_assignment

.var_a:
    mov r14d, 1
    lea r12, [rip+asm_store_a_fmt]
    jmp .check_immutable
.var_b:
    mov r14d, 2
    lea r12, [rip+asm_store_b_fmt]
    jmp .check_immutable
.var_c:
    mov r14d, 4
    lea r12, [rip+asm_store_c_fmt]

.check_immutable:
    test QWORD PTR [rip+set_mask], r14
    jne .bad_assignment
    test r13d, r13d
    je .find_equals
    or QWORD PTR [rip+set_mask], r14

.find_equals:
    mov rdi, rbx
    mov esi, '='
    call strchr
    test rax, rax
    je .bad_assignment
    lea rdi, [rax+1]
    call skip_ws
    mov rbx, rax

    cmp BYTE PTR [rbx], 'a'
    jne .parse_number_assignment
    cmp BYTE PTR [rbx+1], ' '
    jne .bad_assignment
    cmp BYTE PTR [rbx+2], '+'
    jne .bad_assignment
    cmp BYTE PTR [rbx+3], ' '
    jne .bad_assignment
    cmp BYTE PTR [rbx+4], 'b'
    jne .bad_assignment
    lea rdi, [rip+asm_store_c_add]
    mov rsi, QWORD PTR [rip+out_file]
    call fputs
    jmp .assignment_done

.parse_number_assignment:
    movzx eax, BYTE PTR [rbx]
    cmp al, '-'
    je .number_ok
    cmp al, '0'
    jb .bad_assignment
    cmp al, '9'
    ja .bad_assignment
.number_ok:
    mov rdi, rbx
    xor esi, esi
    mov edx, 10
    call strtol

    mov rdi, QWORD PTR [rip+out_file]
    mov rsi, r12
    mov rdx, rax
    xor eax, eax
    call fprintf

.assignment_done:
    pop r14
    pop r13
    pop r12
    pop rbx
    pop rbp
    ret

.bad_assignment:
    mov QWORD PTR [rip+compile_error], 1
    pop r14
    pop r13
    pop r12
    pop rbx
    pop rbp
    ret

emit_return:
    push rbp
    mov rbp, rsp
    push rbx
    sub rsp, 8

    add rdi, 7
    call skip_ws
    mov rbx, rax

    movzx eax, BYTE PTR [rbx]
    cmp al, 'a'
    je .return_a
    cmp al, 'b'
    je .return_b
    cmp al, 'c'
    je .return_c
    cmp al, '-'
    je .parse_return
    cmp al, '0'
    jb .bad_return
    cmp al, '9'
    ja .bad_return

.parse_return:
    mov rdi, rbx
    xor esi, esi
    mov edx, 10
    call strtol

    mov rdi, QWORD PTR [rip+out_file]
    lea rsi, [rip+asm_return_fmt]
    mov rdx, rax
    xor eax, eax
    call fprintf

    add rsp, 8
    pop rbx
    pop rbp
    ret

.bad_return:
    mov QWORD PTR [rip+compile_error], 1
    add rsp, 8
    pop rbx
    pop rbp
    ret

.return_a:
    lea rsi, [rip+asm_return_a]
    jmp .emit_return_var
.return_b:
    lea rsi, [rip+asm_return_b]
    jmp .emit_return_var
.return_c:
    lea rsi, [rip+asm_return_c]
.emit_return_var:
    mov rdi, rsi
    mov rsi, QWORD PTR [rip+out_file]
    call fputs
    add rsp, 8
    pop rbx
    pop rbp
    ret
