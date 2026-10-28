package pki

// Purpose: refuse a node public key of small order (D-1259). ed25519.Verify accepts such a key for any
//          message with a crafted signature, so naming one proves nothing about its holder.
// Inputs:  a raw 32-byte Ed25519 public key.
// Outputs: SmallOrderKey reports whether the encoded point has order dividing 8; ValidNodeKey adds the length check.
// Constraints: pure math/big on the edwards25519 curve (-x^2+y^2 = 1+d*x^2*y^2 over 2^255-19); a
//          non-canonical encoding of y is reduced first, as the verifier does; bytes that are not a curve
//          point are not small order here (Verify refuses them on its own).

import (
	"crypto/ed25519"
	"math/big"
)

var (
	curveP = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	curveD = func() *big.Int {
		inv := new(big.Int).ModInverse(big.NewInt(121666), curveP)
		d := new(big.Int).Mul(big.NewInt(-121665), inv)
		return d.Mod(d, curveP)
	}()
)

// ValidNodeKey is the shape check of a node key: the right length and not a small-order point.
func ValidNodeKey(pub []byte) bool {
	return len(pub) == ed25519.PublicKeySize && !SmallOrderKey(pub)
}

// SmallOrderKey reports whether pub encodes a point whose order divides 8.
func SmallOrderKey(pub []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	le := make([]byte, len(pub))
	for i, b := range pub { // little-endian to big-endian, sign bit cleared
		le[len(pub)-1-i] = b
	}
	le[0] &= 0x7f
	y := new(big.Int).SetBytes(le)
	y.Mod(y, curveP)
	y2 := new(big.Int).Mul(y, y)
	num := new(big.Int).Sub(y2, big.NewInt(1))
	den := new(big.Int).Mul(curveD, y2)
	den.Add(den, big.NewInt(1)).Mod(den, curveP)
	if den.Sign() == 0 {
		return false
	}
	x2 := num.Mul(num, new(big.Int).ModInverse(den, curveP))
	x2.Mod(x2, curveP)
	x := new(big.Int).ModSqrt(x2, curveP)
	if x == nil {
		return false // not a curve point
	}
	px, py := x, y
	for range 3 { // order divides 8 exactly when three doublings reach the identity
		px, py = edAdd(px, py, px, py)
	}
	return px.Sign() == 0 && py.Cmp(big.NewInt(1)) == 0
}

// edAdd is the complete twisted Edwards addition (a = -1) in affine coordinates.
func edAdd(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	xx := new(big.Int).Mul(x1, x2)
	yy := new(big.Int).Mul(y1, y2)
	t := new(big.Int).Mul(xx, yy)
	t.Mul(t, curveD).Mod(t, curveP) // d*x1*x2*y1*y2
	xn := new(big.Int).Add(new(big.Int).Mul(x1, y2), new(big.Int).Mul(x2, y1))
	xd := new(big.Int).Add(big.NewInt(1), t)
	yn := new(big.Int).Add(yy, xx)
	yd := new(big.Int).Sub(big.NewInt(1), t)
	x3 := xn.Mul(xn.Mod(xn, curveP), new(big.Int).ModInverse(xd.Mod(xd, curveP), curveP))
	y3 := yn.Mul(yn.Mod(yn, curveP), new(big.Int).ModInverse(yd.Mod(yd, curveP), curveP))
	return x3.Mod(x3, curveP), y3.Mod(y3, curveP)
}
