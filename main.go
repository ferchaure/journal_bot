// agregar al dict que sea por message id que que responda con las opciones para permitir paralelo
// usar todo colas o algo safe
package main

import (
	"fmt"
)

type user_data struct { //just remember the last message of the user
	Content           string
	File              string
	OriginalMessageID int
	CurrMessageID     int
}

func main() {

	LoadConfig()
	bc := ConfigBot()
	defer bc.ExecuteDefer()
	fmt.Println("Running bot...")
	StartBot(bc)
}
