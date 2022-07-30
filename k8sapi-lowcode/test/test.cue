package test

#input:{
	id: int & >3 | *4
	name?: string
	role: "admin" | "guest" | "developer"
}