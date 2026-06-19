package common

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

type ResolvedDependencies []ResolvedDependency

func (ri ResolvedDependencies) GroupByPackage() map[*FileRange]ResolvedDependencies {
	groups := make(map[*FileRange]ResolvedDependencies)
	for _, imp := range ri {
		groups[imp.Internal] = append(groups[imp.Internal], imp)
	}
	return groups
}
