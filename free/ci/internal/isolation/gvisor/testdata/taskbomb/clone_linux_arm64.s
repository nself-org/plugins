#include "textflag.h"

// Block signals across clone: raw children must never enter Go signal handlers.
TEXT ·cloneTask(SB),NOSPLIT,$32-16
	MOVD $-1, R0
	MOVD R0, 8(RSP)
	MOVD $135, R8 // rt_sigprocmask(SIG_SETMASK, all, &old, 8)
	MOVD $2, R0
	ADD $8, RSP, R1
	ADD $16, RSP, R2
	MOVD $8, R3
	SVC $0
	CBNZ R0, done
	MOVD $220, R8 // clone(CLONE_VM|FS|FILES|SIGHAND|THREAD|SYSVSEM)
	MOVD $0x50f00, R0
	MOVD stack+0(FP), R1
	MOVD ZR, R2
	MOVD ZR, R3
	MOVD ZR, R4
	SVC $0
	CBZ R0, child
	MOVD R0, 24(RSP)
	MOVD $135, R8 // restore the parent's signal mask
	MOVD $2, R0
	ADD $16, RSP, R1
	MOVD ZR, R2
	MOVD $8, R3
	SVC $0
	MOVD 24(RSP), R0
done:
	MOVD R0, ret+8(FP)
	RET
child:
	MOVD ZR, (RSP)
wait:
	MOVD $98, R8 // futex(stack, FUTEX_WAIT_PRIVATE, 0, NULL)
	MOVD RSP, R0
	MOVD $128, R1
	MOVD ZR, R2
	MOVD ZR, R3
	MOVD ZR, R4
	MOVD ZR, R5
	SVC $0
	B wait
