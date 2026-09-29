package axlr

type ExecArgs struct {
	Program        string   `json:"program"`
	Args           []string `json:"args"`
	Cwd            string   `json:"cwd"`
	Stdin          string   `json:"stdin"`
	TimeoutMS      int64    `json:"timeout_ms"`
	MaxOutputBytes int      `json:"max_output_bytes"`
}
