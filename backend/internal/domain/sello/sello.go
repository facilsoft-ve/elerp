// Package sello implementa el SELLO DE INTEGRIDAD de los libros append-only:
// cada registro guarda el hash del registro anterior (PrevHash) y su propio hash
// (Hash = SHA-256 de PrevHash + su contenido canónico). Así, alterar, borrar o
// reordenar cualquier registro rompe la cadena de todos los siguientes — se puede
// DEMOSTRAR ante el SENIAT que el libro no se manipuló (principio 2, §13.2 del doc
// de seguridad). El sello es metadato de integridad; no cambia el dato de negocio.
package sello

import (
	"crypto/sha256"
	"encoding/hex"
)

// Encadenar devuelve el hash del registro: SHA-256 de (prevHash \n contenido). El
// `contenido` es la representación canónica e inmutable del registro (lo produce
// cada agregado con su método Contenido()).
func Encadenar(prevHash, contenido string) string {
	h := sha256.Sum256([]byte(prevHash + "\n" + contenido))
	return hex.EncodeToString(h[:])
}
