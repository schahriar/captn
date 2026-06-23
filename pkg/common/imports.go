package common

import (
	"context"
	"os"
	"path/filepath"
	"runtime/trace"
)

type DependencyType string

const (
	LocalDependency           DependencyType = "local"
	PackageDependency         DependencyType = "package"
	StandardLibraryDependency DependencyType = "stdlib"
)

type ResolvedDependency struct {
	Type     DependencyType
	Internal *FileRange
	External *FileRange
}

func NewResolvedDependency(t DependencyType, i *FileRange, e *FileRange) ResolvedDependency {
	return ResolvedDependency{
		Type:     t,
		Internal: i,
		External: e,
	}
}

func NewResolvedDependencyFromURI(
	ctx context.Context,
	workspace string,
	internal *FileRange,
	uri string,
	startLine int,
	startColumn int,
	endLine int,
	endColumn int,
	classify func(*Source) DependencyType,
) (ResolvedDependency, error) {
	zero := NewResolvedDependency(LocalDependency, nil, nil)
	refp, err := AbsolutePathFromURI(uri)
	if err != nil {
		return zero, err
	}

	rel, err := filepath.Rel(workspace, refp)
	if err != nil {
		return zero, err
	}

	_, task := trace.NewTask(ctx, "loadResolvedDependency")
	defer task.End()

	buf, err := os.ReadFile(refp)
	if err != nil {
		return zero, err
	}

	src := NewSource(workspace, rel, buf)
	external, err := NewFileRangeAutoBytePosition(src, startLine, startColumn, endLine, endColumn)
	if err != nil {
		return zero, err
	}

	dependencyType := LocalDependency
	if classify != nil {
		dependencyType = classify(src)
	}

	return NewResolvedDependency(dependencyType, internal, external), nil
}

type ResolvedDependencies []ResolvedDependency

func (ri ResolvedDependencies) GroupByPackage() map[*FileRange]ResolvedDependencies {
	groups := make(map[*FileRange]ResolvedDependencies)
	for _, imp := range ri {
		groups[imp.Internal] = append(groups[imp.Internal], imp)
	}
	return groups
}
