package platform

import (
	"context"
	"fmt"
	"strconv"
)

// ResolveMinecraft freezes the Java runtime required for a published game version.
func ResolveMinecraft(ctx context.Context, c MinecraftConfig, resolver func(context.Context, string) (int, error)) (MinecraftConfig, error) {
	if err := c.Validate(); err != nil {
		return c, err
	}
	if resolver == nil {
		resolver = javaForVersion
	}
	major, err := resolver(ctx, c.Version)
	if err != nil {
		return c, err
	}
	if c.Java != "auto" {
		selected, _ := strconv.Atoi(c.Java)
		if selected < major {
			return c, fmt.Errorf("selected Java runtime is older than Minecraft requires")
		}
		major = selected
	} else if c.Type == "MODRINTH" && major >= 21 {
		major = 25
	}
	if _, err = minecraftJavaImage(major); err != nil {
		return c, err
	}
	c.JavaMajor = major
	return c, nil
}
