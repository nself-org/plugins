#include "textflag.h"

// Block signals across clone: raw children must never enter Go signal handlers.
TEXT ·cloneTask(SB),NOSPLIT,$24-16
	MOVQ $-1, 0(SP)
	MOVQ $14, AX // rt_sigprocmask(SIG_SETMASK, all, &old, 8)
	MOVQ $2, DI
	LEAQ 0(SP), SI
	LEAQ 8(SP), DX
	MOVQ $8, R10
	SYSCALL
	TESTQ AX, AX
	JNZ done
	MOVQ $56, AX // clone(CLONE_VM|FS|FILES|SIGHAND|THREAD|SYSVSEM)
	MOVQ $0x50f00, DI
	MOVQ stack+0(FP), SI
	XORQ DX, DX
	XORQ R10, R10
	XORQ R8, R8
	SYSCALL
	TESTQ AX, AX
	JZ child
	MOVQ AX, 16(SP)
	MOVQ $14, AX // restore the parent's signal mask
	MOVQ $2, DI
	LEAQ 8(SP), SI
	XORQ DX, DX
	MOVQ $8, R10
	SYSCALL
	MOVQ 16(SP), AX
done:
	MOVQ AX, ret+8(FP)
	RET
child:
	MOVQ $0, 0(SP)
wait:
	MOVQ $202, AX // futex(stack, FUTEX_WAIT_PRIVATE, 0, NULL)
	MOVQ SP, DI
	MOVQ $128, SI
	XORQ DX, DX
	XORQ R10, R10
	XORQ R8, R8
	XORQ R9, R9
	SYSCALL
	JMP wait
