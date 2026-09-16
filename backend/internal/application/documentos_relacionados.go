package application

import (
	"sort"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// DOCUMENTOS RELACIONADOS de un documento fiscal.
//
// Notas de la jornada con la contadora:
//   - «La vista de factura debe mostrar los documentos relacionados por la factura»
//   - «La vista de las notas deben mostrar las facturas relacionadas»
//
// El vínculo YA EXISTÍA en el dato: una nota de crédito, una nota de débito o una
// anulación referencian su original con `RefDocumentoID`, y las retenciones
// referencian el documento sobre el que se retuvo. Lo que faltaba era poder
// RECORRERLO — y en los DOS sentidos, que es lo que pidió la contadora: desde la
// factura hacia lo que salió de ella, y desde una nota de vuelta a su factura.
//
// Sin esto, para saber si una factura tenía nota de crédito había que buscarla a
// mano en la lista, que es justo lo que hace que nadie lo revise.

// DocumentoVinculo es un documento referenciado, resumido para enlazarlo. No se
// devuelve el documento completo a propósito: la vista solo necesita
// identificarlo y poder saltar a él, y arrastrar las líneas de cada relacionado
// haría la respuesta enorme en una factura con varias notas.
type DocumentoVinculo struct {
	ID             string  `json:"id"`
	Tipo           string  `json:"tipo"`
	NumeroCompleto string  `json:"numeroCompleto"`
	Fecha          string  `json:"fecha"`
	Total          float64 `json:"total"`
	Motivo         string  `json:"motivo,omitempty"`
}

// RetencionVinculo es un comprobante de retención sobre el documento, resumido.
type RetencionVinculo struct {
	ID                string  `json:"id"`
	Impuesto          string  `json:"impuesto"` // iva | islr
	NumeroComprobante string  `json:"numeroComprobante"`
	Fecha             string  `json:"fecha"`
	Porcentaje        float64 `json:"porcentaje"`
	MontoRetenido     float64 `json:"montoRetenido"`
	Concepto          string  `json:"concepto,omitempty"`
}

// DocumentosRelacionadosView es lo que la vista de un documento necesita para
// mostrar de dónde viene y qué salió de él.
type DocumentosRelacionadosView struct {
	// Origen es la factura que este documento corrige o complementa. Solo viene
	// en notas de crédito, notas de débito y anulaciones; nil en una factura.
	Origen *DocumentoVinculo `json:"origen,omitempty"`
	// Derivados son los documentos que REFERENCIAN a este: sus notas de crédito,
	// notas de débito y su anulación. Vacío en un documento del que no salió nada.
	Derivados []DocumentoVinculo `json:"derivados"`
	// Retenciones son los comprobantes de retención registrados sobre este
	// documento. Van aparte de `Derivados` porque no son documentos fiscales de
	// venta: no tienen número de control ni entran en el libro de ventas como una
	// nota, y mezclarlos confundiría la lectura.
	Retenciones []RetencionVinculo `json:"retenciones"`
}

// vinculoDe resume un documento para enlazarlo.
func vinculoDe(d fiscal.Documento) DocumentoVinculo {
	return DocumentoVinculo{
		ID: d.ID, Tipo: d.Tipo, NumeroCompleto: d.NumeroCompleto,
		Fecha: d.Fecha, Total: d.Total, Motivo: d.Motivo,
	}
}

// DocumentosRelacionados devuelve el origen, los derivados y las retenciones de
// un documento. Devuelve false si el documento no existe o no es de la empresa —
// el aislamiento por tenant se respeta acá igual que en todas las consultas.
func (s *Service) DocumentosRelacionados(empresaID, documentoID string) (DocumentosRelacionadosView, bool) {
	out := DocumentosRelacionadosView{
		Derivados:   []DocumentoVinculo{},
		Retenciones: []RetencionVinculo{},
	}
	if s.documentos == nil {
		return out, false
	}
	doc, ok := s.documentos.ByID(empresaID, documentoID)
	if !ok {
		return out, false
	}

	// Una sola pasada sobre los documentos de la empresa: el origen y los
	// derivados salen del mismo recorrido. Buscarlos por separado duplicaría el
	// barrido sin ganar nada.
	todos := s.documentos.List(empresaID)
	for _, d := range todos {
		if d.ID == doc.ID {
			continue
		}
		if d.RefDocumentoID == doc.ID {
			out.Derivados = append(out.Derivados, vinculoDe(d))
		}
		if doc.RefDocumentoID != "" && d.ID == doc.RefDocumentoID {
			v := vinculoDe(d)
			out.Origen = &v
		}
	}
	// Del más viejo al más nuevo: en una factura con varias notas, el orden en
	// que ocurrieron es la historia de la corrección.
	sort.SliceStable(out.Derivados, func(i, j int) bool { return out.Derivados[i].Fecha < out.Derivados[j].Fecha })

	if s.retenciones != nil {
		for _, r := range s.retenciones.List(empresaID) {
			if r.DocumentoID != doc.ID {
				continue
			}
			out.Retenciones = append(out.Retenciones, RetencionVinculo{
				ID: r.ID, Impuesto: r.Impuesto, NumeroComprobante: r.NumeroComprobante,
				Fecha: r.Fecha, Porcentaje: r.Porcentaje, MontoRetenido: r.MontoRetenido,
				Concepto: r.Concepto,
			})
		}
		sort.SliceStable(out.Retenciones, func(i, j int) bool { return out.Retenciones[i].Fecha < out.Retenciones[j].Fecha })
	}
	return out, true
}
