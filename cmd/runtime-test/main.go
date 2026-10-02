package main

import (
	"context"
	"fmt"
	"log"

	bnbruntime "buildandbreak/platform/internal/runtime"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Build & Break Image Management Test ===")

	dockerRuntime, err := bnbruntime.NewDockerRuntime()
	if err != nil {
		log.Fatalf("failed to create Docker runtime: %v", err)
	}
	defer dockerRuntime.Close()

	image := "alpine:latest"

	fmt.Printf("\nChecking whether %q exists locally...\n", image)

	exists, err := dockerRuntime.ImageExists(ctx, image)
	if err != nil {
		log.Fatalf("image check failed: %v", err)
	}

	fmt.Printf("Exists before EnsureImage: %t\n", exists)

	fmt.Printf("\nEnsuring %q is available...\n", image)

	if err := dockerRuntime.EnsureImage(ctx, image); err != nil {
		log.Fatalf("ensure image failed: %v", err)
	}

	fmt.Println("EnsureImage completed.")

	fmt.Println("\nChecking image again...")

	exists, err = dockerRuntime.ImageExists(ctx, image)
	if err != nil {
		log.Fatalf("second image check failed: %v", err)
	}

	fmt.Printf("Exists after EnsureImage: %t\n", exists)

	if !exists {
		log.Fatal(
			"TEST FAILED: image still missing after EnsureImage",
		)
	}

	fmt.Println()
	fmt.Println("=== Image Management Test Passed ===")
}
