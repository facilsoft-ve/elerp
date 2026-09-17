// Package sede modela la Sede/tienda: el tercer nivel de la jerarquía. Las
// existencias de inventario y las cajas viven a nivel de sede.
package sede

// Sede es un local físico de una empresa.
type Sede struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Direccion string `json:"direccion" bson:"direccion"`
	// Lat/Lon son la UBICACIÓN del local (grados decimales). Sostienen la
	// «presencia estricta»: ciertos roles solo pueden empezar a trabajar estando
	// en la sede. Cero/cero significa SIN UBICACIÓN configurada — y no es una
	// coordenada real que haya que respetar: el (0,0) cae en el Atlántico, así
	// que usarlo como «sin configurar» no pisa ninguna sede posible.
	Lat float64 `json:"lat" bson:"lat"`
	Lon float64 `json:"lon" bson:"lon"`
	// RadioM es el radio de tolerancia en metros. 0 = usar RadioPorDefecto.
	// Es configurable porque el local grande y el kiosco no se miden igual.
	RadioM int  `json:"radioM" bson:"radiom"`
	Activa bool `json:"activa" bson:"activa"`
}

// RadioPorDefecto son los metros de tolerancia cuando la sede no declara uno.
//
// 150 m y no 20: el GPS BAJO TECHO es malo. Entre paredes, y más en una cocina,
// la precisión se va a 50–100 m o peor. Un radio chico deja afuera a gente que
// está parada dentro del restaurante — y una regla que no se puede cumplir no
// protege, solo se rompe (terminan pidiendo la excepción todas las noches).
const RadioPorDefecto = 150

// TieneUbicacion indica si la sede tiene coordenadas configuradas. Sin ellas no
// hay nada contra qué verificar y la presencia estricta sencillamente no aplica.
func (s Sede) TieneUbicacion() bool { return s.Lat != 0 || s.Lon != 0 }

// RadioEfectivo son los metros de tolerancia que rigen para esta sede.
func (s Sede) RadioEfectivo() int {
	if s.RadioM > 0 {
		return s.RadioM
	}
	return RadioPorDefecto
}

// CoordenadaValida acota lat/lon al rango físico. Existe porque un dedo de más
// al teclear (o un sensor roto) guardaría una sede en una latitud imposible y la
// verificación pasaría a rechazar a todo el mundo sin explicación.
func CoordenadaValida(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// Repository es el puerto de persistencia de sedes. Toda consulta se aísla por
// empresaID (el tenant).
type Repository interface {
	List(empresaID string) []Sede
	ByID(empresaID, id string) (Sede, bool)
	Create(s Sede) Sede
	Update(s Sede) (Sede, bool)
}
