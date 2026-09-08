package application

import (
	"errors"
	"fmt"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// ErrNadaQueCerrar se devuelve al intentar emitir un Cierre Z cuando no hay
// documentos nuevos desde el último Z de la sede: un cierre vacío no es un
// documento fiscal, es ruido.
var ErrNadaQueCerrar = errors.New("no hay documentos por cerrar desde el último Z")

// CierresZ devuelve los cierres Z de una sede, ordenados por Numero ascendente
// (del primero al último). Solo lectura.
func (s *Service) CierresZ(empresaID, sedeID string) []fiscal.CierreZ {
	if s.cierresZ == nil {
		return []fiscal.CierreZ{}
	}
	out := []fiscal.CierreZ{}
	for _, z := range s.cierresZ.List(empresaID) {
		if z.SedeID == sedeID {
			out = append(out, z)
		}
	}
	// Orden por Numero ascendente (insertion sort, como el resto del código).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Numero < out[j-1].Numero; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// docsPorCerrar reúne los documentos de una sede pendientes de cierre: los de
// esa SedeID con Fecha estrictamente posterior al HastaFecha del último Z (o
// todos si aún no hay Z), ordenados por Fecha ascendente.
func (s *Service) docsPorCerrar(empresaID, sedeID string) []fiscal.Documento {
	// Último Z de la sede: el de mayor Numero. Su HastaFecha es el corte
	// (exclusivo) desde el que arranca el nuevo cierre.
	desde := ""
	for _, z := range s.CierresZ(empresaID, sedeID) {
		desde = z.HastaFecha // CierresZ ya viene ordenado ascendente: el último gana
	}
	docs := []fiscal.Documento{}
	for _, d := range s.documentos.List(empresaID) {
		if d.SedeID != sedeID {
			continue
		}
		// desde == "" (no hay Z previo) ⇒ entran todos. Con Z previo, solo los
		// posteriores al corte: la comparación de strings RFC3339 UTC ordena
		// cronológicamente.
		if desde != "" && d.Fecha <= desde {
			continue
		}
		docs = append(docs, d)
	}
	// Orden por Fecha ascendente: el primero y el último definen el rango.
	for i := 1; i < len(docs); i++ {
		for j := i; j > 0 && docs[j].Fecha < docs[j-1].Fecha; j-- {
			docs[j], docs[j-1] = docs[j-1], docs[j]
		}
	}
	return docs
}

// foldZ pliega un conjunto de documentos en un TotalesZ: las facturas suman a
// las ventas y sus impuestos; las notas de crédito y anulaciones suman su monto
// en POSITIVO (los documentos lo guardan negativo). Es la misma proyección
// derivada del ledger que usan los libros fiscales —no un registro paralelo.
func foldZ(docs []fiscal.Documento) fiscal.TotalesZ {
	var t fiscal.TotalesZ
	for _, d := range docs {
		switch d.Tipo {
		case fiscal.TipoFactura, fiscal.TipoNotaDebito:
			// La factura y la nota de débito suman al débito fiscal en POSITIVO. La
			// nota de débito es un cargo adicional (ya inmutable, con su propia base e
			// IVA), así que engrosa las ventas gravadas/exentas y el total del Z; solo
			// las facturas cuentan como documento de venta (CantidadFacturas).
			t.VentasGravadas += d.BaseImponible
			t.VentasExentas += d.BaseExenta
			t.IVADebito += d.IVA
			t.IGTF += d.IGTF
			t.TotalVentas += d.Total
			if d.Tipo == fiscal.TipoFactura {
				t.CantidadFacturas++
			}
		case fiscal.TipoNotaCredito:
			t.NotasCredito += absF(d.Total)
			t.CantidadNotas++
		case fiscal.TipoAnulacion:
			t.Anulaciones += absF(d.Total)
			t.CantidadAnulaciones++
		}
	}
	t.VentasGravadas = round2(t.VentasGravadas)
	t.VentasExentas = round2(t.VentasExentas)
	t.IVADebito = round2(t.IVADebito)
	t.IGTF = round2(t.IGTF)
	t.TotalVentas = round2(t.TotalVentas)
	t.NotasCredito = round2(t.NotasCredito)
	t.Anulaciones = round2(t.Anulaciones)
	t.TotalNeto = round2(t.TotalVentas - t.NotasCredito - t.Anulaciones)
	return t
}

// PreviewCierreZ calcula lo que se cerraría AHORA sin persistir nada: los
// totales del rango pendiente y cuántos documentos incluiría. Sirve para que la
// UI muestre la previsualización antes de confirmar. Con 0 documentos devuelve
// totales en cero y cantidad 0 (sin error): el estado vacío honesto.
func (s *Service) PreviewCierreZ(empresaID, sedeID string) (fiscal.TotalesZ, int, error) {
	docs := s.docsPorCerrar(empresaID, sedeID)
	return foldZ(docs), len(docs), nil
}

// EmitirCierreZ consolida los documentos de la sede pendientes desde el último Z
// en un nuevo Cierre Z inmutable, con numeración Z secuencial por sede. Nunca
// edita un Z previo: la corrección es siempre un Z posterior.
func (s *Service) EmitirCierreZ(empresaID, sedeID, actor, origen string) (fiscal.CierreZ, error) {
	if s.cierresZ == nil {
		return fiscal.CierreZ{}, ErrNadaQueCerrar
	}
	docs := s.docsPorCerrar(empresaID, sedeID)
	if len(docs) == 0 {
		return fiscal.CierreZ{}, ErrNadaQueCerrar
	}

	primero, ultimo := docs[0], docs[len(docs)-1]
	z := fiscal.CierreZ{
		EmpresaID:  empresaID,
		SedeID:     sedeID,
		DesdeFecha: primero.Fecha,
		HastaFecha: ultimo.Fecha,
		DocDesde:   primero.NumeroCompleto,
		DocHasta:   ultimo.NumeroCompleto,
		Totales:    foldZ(docs),
		Fecha:      ahora(),
		Actor:      actor,
	}

	// Numeración Z serializada por empresa+sede (misma mecánica atómica que los
	// folios fiscales, serie "Z").
	z.Numero = s.numerador.Siguiente(empresaID, sedeID, "Z")
	z.NumeroCompleto = fmt.Sprintf("Z-%08d", z.Numero)

	out := s.cierresZ.Append(z)
	if s.audit != nil {
		s.audit.Append(evento(empresaID, actor, origen, "fiscal.cierrez.emitir", out.NumeroCompleto,
			fmt.Sprintf("total neto %.2f", out.Totales.TotalNeto)))
	}
	return out, nil
}

// absF devuelve el valor absoluto de un float64 (evita importar math por una
// resta de signo).
func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
