package entity

type ProjectMap struct {
	Project struct {
		IsDomain    bool   `json:"is_domain"`
		Description string `json:"description"`
		Links       struct {
			Self string `json:"self"`
		} `json:"links"`
		Tags     []interface{} `json:"tags"`
		Enabled  bool          `json:"enabled"`
		Id       string        `json:"id"`
		ParentId string        `json:"parent_id"`
		Options  struct {
		} `json:"options"`
		DomainId string `json:"domain_id"`
		Name     string `json:"name"`
	} `json:"project"`
}