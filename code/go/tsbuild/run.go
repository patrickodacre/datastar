package tsbuild

import (
	"compress/gzip"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/valyala/bytebufferpool"
)

func Build() error {
	version, err := extractVersion()
	if err != nil {
		return fmt.Errorf("error extracting version: %w", err)
	}

	if err := errors.Join(
		// createPluginManifest(),
		createBundles(),
		writeOutConsts(version),
	); err != nil {
		return fmt.Errorf("error creating bundles: %w", err)
	}

	return nil
}

func extractVersion() (string, error) {
	versionFilepath := "VERSION"
	versionBytes, err := os.ReadFile(versionFilepath)
	if err != nil {
		return "", fmt.Errorf("error reading package.json: %w", err)
	}

	version := strings.TrimSpace(string(versionBytes))

	// Write out the version to the version file.
	versionPath := "code/ts/library/src/engine/version.ts"
	versionContents := fmt.Sprintf("export const VERSION = '%s';\n", version)
	if err := os.WriteFile(versionPath, []byte(versionContents), 0644); err != nil {
		return "", fmt.Errorf("error writing version file: %w", err)
	}

	return version, nil
}

func createBundles() error {
	log.Print("Creating bundles...")
	defer log.Print("Bundles created!")

	outDir := "./bundles"
	os.RemoveAll(outDir)

	result := api.Build(api.BuildOptions{
		EntryPoints: []string{
			"code/ts/library/src/bundles/datastar-core.ts",
			"code/ts/library/src/bundles/datastar.ts",
		},
		Outdir:            outDir,
		Bundle:            true,
		Write:             true,
		LogLevel:          api.LogLevelInfo,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Sourcemap:         api.SourceMapLinked,
		Target:            api.ES2023,
	})

	if len(result.Errors) > 0 {
		errs := make([]error, len(result.Errors))
		for i, err := range result.Errors {
			errs[i] = errors.New(err.Text)
		}
		return errors.Join(errs...)
	}

	return nil
}

func writeOutConsts(version string) error {
	log.Print("Extracting version...")

	Consts.Version = version

	build, err := os.ReadFile("bundles/datastar.js")
	if err != nil {
		return fmt.Errorf("error reading datastar.js: %w", err)
	}
	Consts.VersionClientByteSize = len(build)

	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)

	w, err := gzip.NewWriterLevel(buf, gzip.BestCompression)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(build); err != nil {
		panic(err)
	}
	w.Close()
	Consts.VersionClientByteSizeGzip = buf.Len()

	var zeroCased toolbelt.CasedString
	// Make sure all enums are set up.
	for _, enum := range Consts.Enums {
		for _, value := range enum.Values {
			if value.Name == zeroCased {
				value.Name = toolbelt.ToCasedString(value.Value)
			}
		}
		if enum.DefaultIndex >= 0 {
			enum.Default = enum.Values[enum.DefaultIndex]
		}
	}

	slices.SortFunc(Consts.SDKLanguages, func(a, b Language) int {
		return strings.Compare(a.Name, b.Name)
	})

	templates := map[string]func(data *ConstTemplateData) string{
		"README.md":                                    datastarREADME,
		"code/ts/library/README.md":                    datastarREADME,
		"code/ts/library/src/engine/consts.ts":         datastarClientConsts,
		"code/ts/library/package.json":                 datastarClientPackageJSON,
		"code/go/sdk/consts.go":                        goConsts,
		"code/dotnet/sdk/src/Consts.fs":                dotnetConsts,
		"code/php/sdk/src/Consts.php":                  phpConsts,
		"code/php/sdk/src/enums/EventType.php":         phpEventType,
		"code/php/sdk/src/enums/FragmentMergeMode.php": phpFragmentMergeMode,
	}

	for path, tmplFn := range templates {
		log.Printf("Writing %s...", path)
		contents := strings.TrimSpace(tmplFn(Consts))
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			return fmt.Errorf("error writing version file: %w", err)
		}
	}

	return nil
}

func durationToMs(d time.Duration) int {
	return int(d.Milliseconds())
}
