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
