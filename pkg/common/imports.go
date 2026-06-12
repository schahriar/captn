package common

type ImportType string

const (
	ImportLocal           ImportType = "local"
	ImportDependency      ImportType = "dependency"
	ImportStandardLibrary ImportType = "stdlib"
)

type ResolvedImport struct {
	Type     ImportType
	Internal *FileRange
	External *FileRange
}

func NewResolvedImport(t ImportType, i *FileRange, e *FileRange) ResolvedImport {
	return ResolvedImport{
		Type:     t,
		Internal: i,
		External: e,
	}
}

type ResolvedImports []ResolvedImport

func (ri ResolvedImports) GroupByPackage() map[*FileRange]ResolvedImports {
	groups := make(map[*FileRange]ResolvedImports)
	for _, imp := range ri {
		groups[imp.Internal] = append(groups[imp.Internal], imp)
	}
	return groups
}
