// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Version es una versión Major.Minor del protocolo A2A.
type Version string

const (
	V10 Version = "1.0"
	V03 Version = "0.3"
)

// HeaderVersion es el parámetro de servicio con la versión solicitada. Puede
// llegar como cabecera o como parámetro de consulta.
const HeaderVersion = "A2A-Version"

// SupportedVersions son las versiones que el gateway sabe tratar.
var SupportedVersions = []Version{V10, V03}

// ParseVersion interpreta un valor Major.Minor o Major.Minor.Patch. El parche
// no interviene en la negociación (especificación 1.0, sección 3.6).
func ParseVersion(s string) (Version, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return "", false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return "", false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", false
		}
		nums[i] = n
	}
	v := Version(strconv.Itoa(nums[0]) + "." + strconv.Itoa(nums[1]))
	for _, known := range SupportedVersions {
		if v == known {
			return v, true
		}
	}
	return "", false
}

// RequestedVersion devuelve la versión pedida: la cabecera A2A-Version o, si
// falta o está vacía, el parámetro de consulta del mismo nombre, sin distinguir
// mayúsculas. Varios valores distintos se devuelven unidos por comas para que
// Negotiate los rechace.
func RequestedVersion(h http.Header, q url.Values) string {
	if v := distinct(h.Values(HeaderVersion)); v != "" {
		return v
	}
	var vals []string
	for k, vs := range q {
		if strings.EqualFold(k, HeaderVersion) {
			vals = append(vals, vs...)
		}
	}
	return distinct(vals)
}

func distinct(vals []string) string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return strings.Join(out, ",")
}

// Negotiate decide la versión de la petición. Sin valor se asume 0.3, como
// exige la especificación. Una versión desconocida o no habilitada para el
// agente es -32009.
func Negotiate(requested string, enabled []Version) (Version, *Error) {
	v := V03
	if requested != "" {
		parsed, ok := ParseVersion(requested)
		if !ok {
			return "", newError(CodeVersionNotSupported, "Version not supported",
				"versión solicitada no reconocida "+quote(requested))
		}
		v = parsed
	}
	for _, e := range enabled {
		if e == v {
			return v, nil
		}
	}
	detail := "versión " + string(v) + " no habilitada para el agente"
	if requested == "" {
		detail += " (sin A2A-Version se asume 0.3)"
	}
	return "", newError(CodeVersionNotSupported, "Version not supported", detail)
}
