package compiler

// SourceItemContainer flattens the types that carry collected source items:
// whatever the container is, it can hand over its items as one plain list.
type SourceItemContainer interface {
	GetSourceItems() []*SourceItem    // the 'legit' items of this container
	GetAllSourceItems() []*SourceItem // in case there are items of different origins
	GetCurrentProject() *Project      // the project that owns the items
}
