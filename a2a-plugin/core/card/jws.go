// Copyright 2026 Los autores del plugin A2A para KrakenD.

package card

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// KeySet son las claves de confianza para verificar firmas de Agent Cards.
type KeySet struct {
	keys map[string]crypto.PublicKey
}

// LoadKeySet lee un fichero JWKS. La ruta viene de la configuración del
// operador, no de la petición.
func LoadKeySet(path string) (*KeySet, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("no se puede leer el JWKS %s: %w", path, err)
	}
	return ParseKeySet(raw)
}

// ParseKeySet interpreta un documento JWKS con claves RSA, EC (P-256, P-384,
// P-521) u OKP (Ed25519). Cada clave debe tener kid.
func ParseKeySet(raw []byte) (*KeySet, error) {
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("JWKS inválido: %w", err)
	}
	ks := &KeySet{keys: map[string]crypto.PublicKey{}}
	for i, k := range doc.Keys {
		if k.Kid == "" {
			return nil, fmt.Errorf("JWKS: la clave %d no tiene kid", i)
		}
		var pub crypto.PublicKey
		var err error
		switch k.Kty {
		case "RSA":
			pub, err = rsaKey(k.N, k.E)
		case "EC":
			pub, err = ecKey(k.Crv, k.X, k.Y)
		case "OKP":
			pub, err = okpKey(k.Crv, k.X)
		default:
			err = fmt.Errorf("tipo de clave no admitido %q", k.Kty)
		}
		if err != nil {
			return nil, fmt.Errorf("JWKS: clave %q: %w", k.Kid, err)
		}
		ks.keys[k.Kid] = pub
	}
	if len(ks.keys) == 0 {
		return nil, errors.New("JWKS sin claves")
	}
	return ks, nil
}

func rsaKey(n, e string) (crypto.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}
	exp := new(big.Int).SetBytes(eb)
	if !exp.IsInt64() || exp.Int64() > math.MaxInt32 || exp.Int64() < 3 {
		return nil, errors.New("exponente RSA inválido")
	}
	if len(nb)*8 < 2048 {
		return nil, errors.New("clave RSA de menos de 2048 bits")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(exp.Int64())}, nil
}

func okpKey(crv, x string) (crypto.PublicKey, error) {
	if crv != "Ed25519" {
		return nil, fmt.Errorf("curva OKP no admitida %q", crv)
	}
	xb, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, err
	}
	if len(xb) != ed25519.PublicKeySize {
		return nil, errors.New("clave Ed25519 de tamaño incorrecto")
	}
	return ed25519.PublicKey(xb), nil
}

func ecKey(crv, x, y string) (crypto.PublicKey, error) {
	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("curva no admitida %q", crv)
	}
	xb, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(y)
	if err != nil {
		return nil, err
	}
	pub := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}
	if !curve.IsOnCurve(pub.X, pub.Y) {
		return nil, errors.New("el punto no está en la curva")
	}
	return pub, nil
}

// ErrNoValidSignature indica que la card no tiene ninguna firma válida.
var ErrNoValidSignature = errors.New("la agent card no tiene ninguna firma válida con las claves de confianza")

// VerifyCard comprueba que al menos una firma de la card sea válida, según
// A2A 1.0 sección 8.4 y como lo hace el SDK oficial: JWS con contenido
// separado, sobre la forma canónica RFC 8785 de la card sin el campo
// signatures.
func (ks *KeySet) VerifyCard(raw []byte) error {
	card, err := decodeCard(raw)
	if err != nil {
		return err
	}
	sigs, _ := card["signatures"].([]interface{})
	if len(sigs) == 0 {
		return fmt.Errorf("%w: la card no está firmada", ErrNoValidSignature)
	}
	delete(card, "signatures")
	payload, err := Canonicalize(card)
	if err != nil {
		return err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)

	var reasons []string
	for i, s := range sigs {
		if err := ks.verifyOne(s, payloadB64); err != nil {
			reasons = append(reasons, fmt.Sprintf("firma %d: %v", i, err))
			continue
		}
		return nil
	}
	return fmt.Errorf("%w (%s)", ErrNoValidSignature, strings.Join(reasons, "; "))
}

func (ks *KeySet) verifyOne(s interface{}, payloadB64 string) error {
	obj, ok := s.(map[string]interface{})
	if !ok {
		return errors.New("no es un objeto")
	}
	protectedB64, _ := obj["protected"].(string)
	sigB64, _ := obj["signature"].(string)
	if protectedB64 == "" || sigB64 == "" {
		return errors.New("faltan protected o signature")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(protectedB64)
	if err != nil {
		return errors.New("protected no es base64url")
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return errors.New("protected no es JSON")
	}
	alg, _ := header["alg"].(string)
	kid, _ := header["kid"].(string)
	if crit, ok := header["crit"]; ok && crit != nil {
		return errors.New("cabecera crit no admitida")
	}
	key, ok := ks.keys[kid]
	if !ok {
		return fmt.Errorf("kid %q no está entre las claves de confianza", kid)
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return errors.New("signature no es base64url")
	}
	return verifySignature(key, alg, []byte(protectedB64+"."+payloadB64), sig)
}

var errBadSignature = errors.New("firma inválida")

func verifySignature(key crypto.PublicKey, alg string, msg, sig []byte) error {
	switch k := key.(type) {
	case ed25519.PublicKey:
		return verifyEd25519(k, alg, msg, sig)
	case *ecdsa.PublicKey:
		return verifyECDSA(k, alg, msg, sig)
	case *rsa.PublicKey:
		return verifyRSA(k, alg, msg, sig)
	default:
		return errors.New("tipo de clave no admitido")
	}
}

func verifyEd25519(k ed25519.PublicKey, alg string, msg, sig []byte) error {
	if alg != "EdDSA" {
		return fmt.Errorf("alg %q no corresponde a una clave Ed25519", alg)
	}
	if !ed25519.Verify(k, msg, sig) {
		return errBadSignature
	}
	return nil
}

func verifyECDSA(k *ecdsa.PublicKey, alg string, msg, sig []byte) error {
	want, h := ecdsaAlg(k.Curve)
	if alg != want {
		return fmt.Errorf("alg %q no corresponde a la curva de la clave (%s)", alg, want)
	}
	size := (k.Curve.Params().BitSize + 7) / 8
	if len(sig) != 2*size {
		return errors.New("firma ECDSA de tamaño incorrecto")
	}
	r := new(big.Int).SetBytes(sig[:size])
	s := new(big.Int).SetBytes(sig[size:])
	if !ecdsa.Verify(k, digest(h, msg), r, s) {
		return errBadSignature
	}
	return nil
}

// ecdsaAlg devuelve el alg JWS y el hash que corresponden a la curva.
func ecdsaAlg(curve elliptic.Curve) (string, crypto.Hash) {
	switch curve {
	case elliptic.P256():
		return "ES256", crypto.SHA256
	case elliptic.P384():
		return "ES384", crypto.SHA384
	default:
		return "ES512", crypto.SHA512
	}
}

var rsaHashes = map[string]crypto.Hash{
	"RS256": crypto.SHA256,
	"RS384": crypto.SHA384,
	"RS512": crypto.SHA512,
}

func verifyRSA(k *rsa.PublicKey, alg string, msg, sig []byte) error {
	h, ok := rsaHashes[alg]
	if !ok {
		return fmt.Errorf("alg %q no admitido con una clave RSA", alg)
	}
	if err := rsa.VerifyPKCS1v15(k, h, digest(h, msg), sig); err != nil {
		return errBadSignature
	}
	return nil
}

func digest(h crypto.Hash, msg []byte) []byte {
	switch h {
	case crypto.SHA384:
		s := sha512.Sum384(msg)
		return s[:]
	case crypto.SHA512:
		s := sha512.Sum512(msg)
		return s[:]
	default:
		s := sha256.Sum256(msg)
		return s[:]
	}
}

// Canonicalize serializa un valor JSON decodificado (con json.Number) según
// RFC 8785 (JCS).
func Canonicalize(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJCS(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJCS(buf *bytes.Buffer, v interface{}) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(val))
	case json.Number:
		f, err := strconv.ParseFloat(string(val), 64)
		if err != nil {
			return fmt.Errorf("número inválido %q", val)
		}
		return writeJCSNumber(buf, f)
	case float64:
		return writeJCSNumber(buf, val)
	case string:
		writeJCSString(buf, val)
	case []interface{}:
		return writeJCSArray(buf, val)
	case map[string]interface{}:
		return writeJCSObject(buf, val)
	default:
		return fmt.Errorf("tipo JSON no admitido %T", v)
	}
	return nil
}

func writeJCSNumber(buf *bytes.Buffer, f float64) error {
	s, err := es6Number(f)
	if err != nil {
		return err
	}
	buf.WriteString(s)
	return nil
}

func writeJCSArray(buf *bytes.Buffer, list []interface{}) error {
	buf.WriteByte('[')
	for i, e := range list {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeJCS(buf, e); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeJCSObject(buf *bytes.Buffer, obj map[string]interface{}) error {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	// RFC 8785: orden por unidades de código UTF-16.
	sort.Slice(keys, func(i, j int) bool { return lessUTF16(keys[i], keys[j]) })
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeJCSString(buf, k)
		buf.WriteByte(':')
		if err := writeJCS(buf, obj[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func writeJCSString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// es6Number formatea como Number.prototype.toString de ECMAScript, que es lo
// que exige RFC 8785.
func es6Number(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", errors.New("JCS no admite NaN ni infinito")
	}
	if f == 0 {
		return "0", nil
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		mant, exp, _ := strings.Cut(s, "e")
		sign, digits := exp[:1], strings.TrimLeft(exp[1:], "0")
		return mant + "e" + sign + digits, nil
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}
