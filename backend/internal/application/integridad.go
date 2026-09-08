package application

import (
	"sort"

	"github.com/mornix/elerp/internal/domain/sello"
)

// Verificación del SELLO DE INTEGRIDAD (hash-encadenado) de los libros append-only.
// Recalcula la cadena y detecta CUALQUIER alteración: si el contenido de un registro
// cambió, su hash ya no coincide; si se borró o reordenó un registro, el PrevHash
// del siguiente deja de enlazar. Es la prueba, ante el SENIAT, de que el libro no
// se manipuló desde que se activó el sello (§13.2).

// SelloResultado es el veredicto de integridad de un libro.
type SelloResultado struct {
	Libro     string `json:"libro"`     // "diario" | "documentos"
	Integro   bool   `json:"integro"`   // la cadena sellada verifica de punta a punta
	Total     int    `json:"total"`     // registros del libro
	Sellados  int    `json:"sellados"`  // registros con sello verificado
	SinSellar int    `json:"sinSellar"` // registros anteriores a la activación del sello
	RotoEn    string `json:"rotoEn"`    // primer registro con la cadena rota ("" si íntegro)
}

// VerificarSelloAsientos recalcula el sello del libro diario (orden por Número).
func (s *Service) VerificarSelloAsientos(empresaID string) SelloResultado {
	as := s.LibroDiario(empresaID)
	sort.Slice(as, func(i, j int) bool { return as[i].Numero < as[j].Numero })
	res := SelloResultado{Libro: "diario", Integro: true, Total: len(as)}
	prevSellado := ""
	for _, a := range as {
		if a.Hash == "" {
			res.SinSellar++
			continue
		}
		if sello.Encadenar(a.PrevHash, a.Contenido()) != a.Hash {
			res.Integro, res.RotoEn = false, a.Codigo
			return res
		}
		if res.Sellados > 0 && a.PrevHash != prevSellado {
			res.Integro, res.RotoEn = false, a.Codigo
			return res
		}
		prevSellado = a.Hash
		res.Sellados++
	}
	return res
}

// VerificarSelloDocumentos recalcula el sello del libro de documentos fiscales
// (orden de anexado, que es el orden de la cadena).
func (s *Service) VerificarSelloDocumentos(empresaID string) SelloResultado {
	docs := s.documentos.List(empresaID)
	res := SelloResultado{Libro: "documentos", Integro: true, Total: len(docs)}
	prevSellado := ""
	for _, d := range docs {
		if d.Hash == "" {
			res.SinSellar++
			continue
		}
		if sello.Encadenar(d.PrevHash, d.Contenido()) != d.Hash {
			res.Integro, res.RotoEn = false, etiquetaDoc(d.NumeroCompleto)
			return res
		}
		if res.Sellados > 0 && d.PrevHash != prevSellado {
			res.Integro, res.RotoEn = false, etiquetaDoc(d.NumeroCompleto)
			return res
		}
		prevSellado = d.Hash
		res.Sellados++
	}
	return res
}

func etiquetaDoc(numero string) string {
	if numero == "" {
		return "(sin número)"
	}
	return numero
}

// VerificarIntegridad devuelve el veredicto de ambos libros.
func (s *Service) VerificarIntegridad(empresaID string) []SelloResultado {
	return []SelloResultado{
		s.VerificarSelloAsientos(empresaID),
		s.VerificarSelloDocumentos(empresaID),
	}
}
