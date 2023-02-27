package utils

import (
	"github.com/eiannone/keyboard"
	"log"
	"os"
)

func PauseExit() {
	log.Println("Press any to exit")
	keyboard.GetSingleKey()
	os.Exit(0)
}
