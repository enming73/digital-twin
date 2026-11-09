package mes

type Metadata struct {
	Description string `json:"description"`
	Synthetic   bool   `json:"synthetic"`
}

func LookupWorkOrder(id string) (Metadata, bool) {
	if id != "work-order-1" {
		return Metadata{}, false
	}
	return Metadata{Description: "Synthetic SMT work order", Synthetic: true}, true
}
