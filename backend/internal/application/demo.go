package application

// Identidad demo usada por el modo DEV_LOGIN y por el seed de datos. Mantener
// en sync: el seed crea el usuario y la membresía de este principal para que
// "Entrar en modo demo" tenga una empresa con datos al iniciar sesión.
//
// La captación de solicitudes de demo NO vive acá: es del embudo comercial de Mornix
// (prospectos, no tenants) y la atiende la consola de plataforma en
// POST /papi/public/leads, con su bandeja para el equipo. El `demolead` que había en el
// core quedó retirado: nunca se usó (ninguna pantalla lo llamaba y no había forma de
// leer lo capturado).
const (
	DemoUserID = "usr_demo"
	DemoNombre = "María Fernández"
	DemoEmail  = "maria@bodegalacima.com"
)
