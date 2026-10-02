package platform

import "testing"

func TestValidateService(t *testing.T) {
	base := CreateServiceRequest{ServerID: "1d1029df-f543-467f-84c4-20d7759a0622", Name: "test", Template: "alpine-test", MemoryMB: 64, CPUThreads: 1}
	if err := validateService(base); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*CreateServiceRequest)
	}{
		{"invalid UUID", func(r *CreateServiceRequest) { r.ServerID = "bad" }},
		{"blank name", func(r *CreateServiceRequest) { r.Name = " " }},
		{"unknown template", func(r *CreateServiceRequest) { r.Template = "arbitrary" }},
		{"invalid CPU", func(r *CreateServiceRequest) { r.CPUThreads = 0 }},
		{"Alpine port", func(r *CreateServiceRequest) { r.Port = 25565 }},
		{"Minecraft EULA", func(r *CreateServiceRequest) { r.Template = "minecraft-java"; r.MemoryMB = 2048; r.Port = 25565 }},
		{"Minecraft memory", func(r *CreateServiceRequest) { r.Template = "minecraft-java"; r.AcceptEULA = true; r.Port = 25565 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.change(&req)
			if validateService(req) == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
	base.Template = "minecraft-java"
	base.MemoryMB = 2048
	base.Port = 25565
	base.AcceptEULA = true
	if err := validateService(base); err != nil {
		t.Fatal(err)
	}
}
