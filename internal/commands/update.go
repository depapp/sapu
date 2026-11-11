package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/depapp/sapu/internal/ui"
	"github.com/depapp/sapu/internal/update"
)

// RunUpdate implements `sapu update [--check]`. It needs no phone.
func RunUpdate(ctx context.Context, current string, check bool) error {
	rel, err := update.Latest(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	fmt.Printf("sapu %s · latest %s\n", current, rel.Tag)
	if !update.Newer(rel.Tag, current) {
		fmt.Println(ui.Green.Render("Already up to date."))
		return nil
	}
	if strings.HasSuffix(current, "-dev") {
		fmt.Println(ui.Gray.Render("Development build; the release binary is at " + rel.URL))
		if !check {
			fmt.Println(ui.Gray.Render("Rebuild from source with `go install github.com/depapp/sapu@latest` instead, or rerun with a release build."))
		}
		return nil
	}
	if check {
		fmt.Printf("Update available: %s\nRun `sapu update` to install it.\n", rel.URL)
		return nil
	}
	fmt.Printf("Downloading %s %s...\n", update.AssetName(), rel.Tag)
	path, err := update.Install(ctx, rel)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	fmt.Printf("%s Updated %s to %s\n", ui.Green.Render(ui.IconSel), path, rel.Tag)
	return nil
}
