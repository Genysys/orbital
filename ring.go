// Copyright (c) 2017 Clearmatics Technologies Ltd

// SPDX-License-Identifier: LGPL-3.0+

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
)

// A Ring is a number of public/private key pairs
type Ring struct {
	PubKeys  []CurvePoint `json:"pubkeys"`
	PrivKeys []*big.Int   `json:"privkeys"`
}


func convert(data []byte) *big.Int {
	z := new(big.Int)
	z.SetBytes(data)
	return z
}


var curveB = new(big.Int).SetInt64(3)


// Bytes converts a public key x,y pair slice to bytes
func (r Ring) Bytes() []byte {
	var b []byte

	for i := 0; i < len(r.PubKeys); i++ {
		b = append(b, r.PubKeys[i].Marshal()...)
	}

	return b
}

// Generate creates public and private keypairs for a ring with the size of n
func (r *Ring) Generate(n int) error {
	for i := 0; i < n; i++ {
		public, private, err := generateKeyPair()
		if err != nil {
			return err
		}
		r.PrivKeys = append(r.PrivKeys, private)
		r.PubKeys = append(r.PubKeys, *public)
	}

	return nil
}

// PubKeyIndex returns the index of a public key
func (r *Ring) PubKeyIndex(pk CurvePoint) int {

	for i, pub := range r.PubKeys {
		if pub == pk {
			return i
		}
	}

	return -1

}

// Signature generates a signature
func (r *Ring) Signature(pk *big.Int, message []byte, signer int) (*RingSignature, error) {
	N := CurvePoint{}.Order() //group.N

	mR := r.Bytes()
	byteslist := append(mR, message...)
	hashp := NewCurvePointFromHash(byteslist)
	// TODO: checkk hashp

	pk.Mod(pk, N)
	hashSP := hashp.ScalarMult(pk)

	n := len(r.PubKeys)
	var ctlist []*big.Int   //This has to be 2n so here we have n = 4 so 2n = 8 :)
	var a, b CurvePoint
	var ri *big.Int
	var e error
	csum := big.NewInt(0)

	for j := 0; j < n; j++ {

		if j != signer {
			// XXX: can be 0
			cj, err := rand.Int(rand.Reader, N) // this returns *big.Int
			if err != nil {
				return nil, err
			}
			// XXX: can be 0!
			tj, err := rand.Int(rand.Reader, N) // this returns *big.Int tooo
			if err != nil {
				return nil, err
			}

			a = r.PubKeys[j].ParameterPointAdd(tj, cj)

			b = hashp.HashPointAdd(hashSP, tj, cj)
			ctlist = append(ctlist, cj)
			ctlist = append(ctlist, tj)
			csum.Add(csum, cj)
		}

		if j == signer {
			dummy := big.NewInt(0)
			ctlist = append(ctlist, dummy)
			ctlist = append(ctlist, dummy)
			ri, e = rand.Int(rand.Reader, N)
			if e != nil {
				return nil, e
			}
			a = CurvePoint{}.ScalarBaseMult(ri)
			b = hashp.ScalarMult(ri)
		}
		byteslist = append(byteslist, a.Marshal()...)
		byteslist = append(byteslist, b.Marshal()...)
	}

	hasha := sha256.Sum256(byteslist)
	hashb := new(big.Int).SetBytes(hasha[:])
	hashb.Mod(hashb, N)
	csum.Mod(csum, N)
	c := new(big.Int).Sub(hashb, csum)
	c.Mod(c, N)

	cx := new(big.Int).Mul(c, pk)
	cx.Mod(cx, N)
	ti := new(big.Int).Sub(ri, cx)
	ti.Mod(ti, N)
	ctlist[2*signer] = c
	ctlist[2*signer+1] = ti

	return &RingSignature{hashSP, ctlist}, nil
}

// Signatures generates a signature given a message
func (r *Ring) Signatures(message []byte) ([]RingSignature, error) {

	var signaturesArr []RingSignature

	for i, privKey := range r.PrivKeys {
		//pub := CurvePoint{}.ScalarBaseMult(privKey)
		pub := r.PubKeys[i]
		signerNumber := r.PubKeyIndex(pub)
		signature, err := r.Signature(privKey, message, signerNumber)
		if err != nil {
			return nil, err
		}
		signaturesArr = append(signaturesArr, *signature)
	}

	return signaturesArr, nil
}

// VerifySignature verifys a signature given a message
func (r *Ring) VerifySignature(message []byte, sigma RingSignature) bool {
	// ring verification
	// assumes R = pk1, pk2, ..., pkn
	// sigma = H(m||R)^x_i, c1, t1, ..., cn, tn = taux, tauy, c1, t1, ..., cn, tn
	tau := sigma.Tau
	ctlist := sigma.Ctlist
	n := len(r.PubKeys)
	N := CurvePoint{}.Order() //group.N

	mR := r.Bytes()
	byteslist := append(mR, message...)
	hashp := NewCurvePointFromHash(byteslist)
	// TODO: check hashp

	csum := big.NewInt(0)

	for j := 0; j < n; j++ {
		cj := ctlist[2*j]
		tj := ctlist[2*j+1]
		cj.Mod(cj, N)
		tj.Mod(tj, N)
		H := hashp.ScalarMult(tj)             //H(m||R)^t
		gt := CurvePoint{}.ScalarBaseMult(tj) //g^t
		yc := r.PubKeys[j].ScalarMult(cj)     // y^c = g^(xc)
		tauc := tau.ScalarMult(cj)            //H(m||R)^(xc)
		gt = gt.Add(yc)
		H = H.Add(tauc) // fieldJacobianToBigAffine `normalizes' values before returning so yes - normalize uses fast reduction using specialised form of secp256k1's prime! :D
		byteslist = append(byteslist, gt.Marshal()...)
		byteslist = append(byteslist, H.Marshal()...)
		csum.Add(csum, cj)
	}

	hash := sha256.Sum256(byteslist)
	hashhash := new(big.Int).SetBytes(hash[:])

	hashhash.Mod(hashhash, N)
	csum.Mod(csum, N)
	if csum.Cmp(hashhash) == 0 {
		return true
	}
	return false
}
