.section .text
skip_ws:
    mov rax, rdi
.skip_loop:
    movzx ecx, BYTE PTR [rax]
    cmp cl, ' '
    je .skip_one
    cmp cl, 9
    je .skip_one
    ret
.skip_one:
    inc rax
    jmp .skip_loop
