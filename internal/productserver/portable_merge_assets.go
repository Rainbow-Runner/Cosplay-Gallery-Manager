package productserver

import (
	"errors"
	"os"
	"path/filepath"
)

type portableMergeAssetPublication struct {
	mergeID, coserRoot, stageRoot, rollbackRoot string
	newCosers, replaceCosers                    []string
	publishedNew, publishedReplace              []string
}

func (p *portableMergeAssetPublication) Publish() error {
	for _, uuid := range p.newCosers {
		source, destination := filepath.Join(p.stageRoot, uuid), filepath.Join(p.coserRoot, uuid)
		if _, err := os.Lstat(destination); err == nil {
			return errors.New("portable merge Coser asset destination already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(source, destination); err != nil {
			return err
		}
		p.publishedNew = append(p.publishedNew, uuid)
	}
	for _, uuid := range p.replaceCosers {
		sourceAssets := filepath.Join(p.stageRoot, uuid, "assets")
		destinationRoot := filepath.Join(p.coserRoot, uuid)
		destinationAssets := filepath.Join(destinationRoot, "assets")
		rollbackDirectory := filepath.Join(p.rollbackRoot, uuid)
		if err := os.MkdirAll(rollbackDirectory, 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(destinationRoot, 0o700); err != nil {
			return err
		}
		if info, err := os.Lstat(destinationRoot); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("portable merge Coser asset root is unsafe")
		}
		if info, err := os.Lstat(sourceAssets); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("portable merge staged Coser assets are unsafe")
		}
		if info, err := os.Lstat(destinationAssets); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("portable merge existing Coser assets are unsafe")
			}
			if err := os.Rename(destinationAssets, filepath.Join(rollbackDirectory, "assets")); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(filepath.Join(rollbackDirectory, "no-assets"), []byte("\n"), 0o600); err != nil {
				return err
			}
		} else {
			return err
		}
		p.publishedReplace = append(p.publishedReplace, uuid)
		if err := os.Rename(sourceAssets, destinationAssets); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destinationRoot, portableImportOwnerMarker), []byte(p.mergeID+"\n"), 0o600); err != nil {
			return err
		}
	}
	return syncPortableDirectory(p.coserRoot)
}

func (p *portableMergeAssetPublication) Rollback() {
	for _, uuid := range p.publishedNew {
		_ = os.RemoveAll(filepath.Join(p.coserRoot, uuid))
	}
	for index := len(p.publishedReplace) - 1; index >= 0; index-- {
		uuid := p.publishedReplace[index]
		destinationRoot := filepath.Join(p.coserRoot, uuid)
		_ = os.RemoveAll(filepath.Join(destinationRoot, "assets"))
		oldAssets := filepath.Join(p.rollbackRoot, uuid, "assets")
		if _, err := os.Lstat(oldAssets); err == nil {
			_ = os.Rename(oldAssets, filepath.Join(destinationRoot, "assets"))
		}
		_ = os.Remove(filepath.Join(destinationRoot, portableImportOwnerMarker))
	}
	_ = os.RemoveAll(p.rollbackRoot)
}

func (p *portableMergeAssetPublication) Finalize() error {
	directories := make([]string, 0, len(p.publishedNew)+len(p.publishedReplace))
	for _, uuid := range append(append([]string{}, p.publishedNew...), p.publishedReplace...) {
		directories = append(directories, filepath.Join(p.coserRoot, uuid))
	}
	if err := removePortableImportMarkers(directories, p.mergeID); err != nil {
		return err
	}
	return os.RemoveAll(p.rollbackRoot)
}
