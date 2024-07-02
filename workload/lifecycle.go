package main

import (
	"context"
	"fmt"
	"os"
	"time"

	sdk "github.com/formancehq/formance-sdk-go/v2"
)

func waitServicesReady(ctx context.Context, client *sdk.Formance) {
	fmt.Println("Waiting for services to be ready")
	waitingServicesCtx, cancel := context.WithDeadline(ctx, time.Now().Add(30*time.Second))
	defer cancel()

	for {
		select {
		case <-time.After(time.Second):
			fmt.Println("Trying to get info of the ledger...")
			_, err := client.Ledger.GetInfo(ctx)
			if err != nil {
				fmt.Printf("error pinging ledger: %s\r\n", err)
				continue
			}
			return
		case <-waitingServicesCtx.Done():
			fmt.Printf("timeout waiting for services to be ready\r\n")
			os.Exit(1)
		}
	}
}
