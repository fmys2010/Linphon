package mg

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultRegionsFallbackCSV = "AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US"
	regionsCatalogRelativePath = "regions.txt"
)

func defaultRegionsCSV(repoRoot string) string {
	regions, err := readRegionCatalog(filepath.Join(repoRoot, regionsCatalogRelativePath))
	if err != nil || len(regions) == 0 {
		return defaultRegionsFallbackCSV
	}

	return strings.Join(regions, ",")
}

func readRegionCatalog(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	regions := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		regions = append(regions, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return regions, nil
}

func isKnownRegion(repoRoot, region string) bool {
	for _, known := range strings.Split(defaultRegionsCSV(repoRoot), ",") {
		if region == strings.TrimSpace(known) {
			return true
		}
	}

	return false
}
