// Package identity resuelve el usuario efectivo bajo el UID arbitrario de OpenShift.
package identity

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Usuario devuelve el nombre del usuario efectivo; el UID numérico si no tiene
// nombre asignado.
//
// OpenShift arranca los contenedores con un UID ARBITRARIO que no existe en
// /etc/passwd, así que os/user falla en ese caso (y además arrastra cgo, que
// impediría compilar un binario estático). Se lee el fichero directamente.
func Usuario() string {
	uid := os.Getuid()

	fichero, err := os.Open("/etc/passwd")
	if err != nil {
		return "uid=" + strconv.Itoa(uid)
	}
	defer fichero.Close()

	buscado := strconv.Itoa(uid)
	escaner := bufio.NewScanner(fichero)
	for escaner.Scan() {
		// Formato: nombre:contraseña:uid:gid:comentario:home:shell
		campos := strings.Split(escaner.Text(), ":")
		if len(campos) >= 3 && campos[2] == buscado {
			return campos[0]
		}
	}
	return "uid=" + buscado
}
