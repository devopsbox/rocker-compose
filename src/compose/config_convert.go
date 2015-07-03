package compose

import (
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/go-yaml/yaml"
)

func NewContainerFromDocker(dockerContainer *docker.Container) (*Container, error) {
	config, err := NewContainerConfigFromDocker(dockerContainer)
	if err != nil {
		return nil, err
	}
	return &Container{
		Id:      dockerContainer.ID,
		Image:   NewImageNameFromString(dockerContainer.Config.Image),
		ImageId: dockerContainer.Image,
		Name:    NewContainerNameFromString(dockerContainer.Name),
		Created: dockerContainer.Created,
		State: &ContainerState{
			Running:    dockerContainer.State.Running,
			Paused:     dockerContainer.State.Paused,
			Restarting: dockerContainer.State.Restarting,
			OOMKilled:  dockerContainer.State.OOMKilled,
			Pid:        dockerContainer.State.Pid,
			ExitCode:   dockerContainer.State.ExitCode,
			Error:      dockerContainer.State.Error,
			StartedAt:  dockerContainer.State.StartedAt,
			FinishedAt: dockerContainer.State.FinishedAt,
		},
		Config:    config,
		container: dockerContainer,
	}, nil
}

func NewContainerConfigFromDocker(apiContainer *docker.Container) (*ConfigContainer, error) {
	yamlData, ok := apiContainer.Config.Labels["rocker-compose-config"]
	if !ok {
		return nil, fmt.Errorf("Expecting container to have label 'rocker-compose-config' to parse it")
	}

	container := &ConfigContainer{}

	if err := yaml.Unmarshal([]byte(yamlData), container); err != nil {
		return nil, fmt.Errorf("Failed to parse YAML config for container %s, error: %s", apiContainer.Name, err)
	}

	if container.Labels != nil {
		for k, _ := range container.Labels {
			if strings.HasPrefix(k, "rocker-compose-") {
				delete(container.Labels, k)
			}
		}
	}

	return container, nil
}

func (config *ConfigContainer) GetApiConfig() *docker.Config {
	// Copy simple values
	apiConfig := &docker.Config{
		Entrypoint: config.Entrypoint,
		Labels:     config.Labels,
	}
	if config.Cmd != nil {
		apiConfig.Cmd = config.Cmd.Parts
	}
	if config.Image != nil {
		apiConfig.Image = *config.Image
	}
	if config.Hostname != nil {
		apiConfig.Hostname = *config.Hostname
	}
	if config.Domainname != nil {
		apiConfig.Domainname = *config.Domainname
	}
	if config.Workdir != nil {
		apiConfig.WorkingDir = *config.Workdir
	}
	if config.User != nil {
		apiConfig.User = *config.User
	}
	if config.Memory != nil {
		apiConfig.Memory = config.Memory.Int64()
	}
	if config.MemorySwap != nil {
		apiConfig.MemorySwap = config.MemorySwap.Int64()
	}
	if config.CpusetCpus != nil {
		apiConfig.CPUSet = *config.CpusetCpus
	}
	if config.CpuShares != nil {
		apiConfig.CPUShares = *config.CpuShares
	}
	if config.NetworkDisabled != nil {
		apiConfig.NetworkDisabled = *config.NetworkDisabled
	}

	// expose
	if config.Expose != nil {
		apiConfig.ExposedPorts = map[docker.Port]struct{}{}
		for _, portBinding := range config.Expose {
			port := (docker.Port)(portBinding)
			apiConfig.ExposedPorts[port] = struct{}{}
		}
	}

	// env
	if config.Env != nil {
		apiConfig.Env = []string{}
		for key, val := range config.Env {
			apiConfig.Env = append(apiConfig.Env, fmt.Sprintf("%s=%s", key, val))
		}
	}

	// volumes
	if config.Volumes != nil {
		hostVolumes := map[string]struct{}{}
		for _, volume := range config.Volumes {
			if !strings.Contains(volume, ":") {
				hostVolumes[volume] = struct{}{}
			}
		}
		if len(hostVolumes) > 0 {
			apiConfig.Volumes = hostVolumes
		}
	}

	// TODO: SecurityOpts, OnBuild ?

	return apiConfig
}

func (config *ConfigContainer) GetApiHostConfig() *docker.HostConfig {
	// TODO: CapAdd, CapDrop, LxcConf, Devices, LogConfig, ReadonlyRootfs,
	//       SecurityOpt, CgroupParent, CPUQuota, CPUPeriod
	// TODO: where Memory and MemorySwap should go?
	hostConfig := &docker.HostConfig{
		DNS:           config.Dns,
		ExtraHosts:    config.AddHost,
		RestartPolicy: config.Restart.ToDockerApi(),
		Memory:        config.Memory.Int64(),
		MemorySwap:    config.MemorySwap.Int64(),
	}

	if config.Net != nil {
		hostConfig.NetworkMode = *config.Net
	}
	if config.Pid != nil {
		hostConfig.PidMode = *config.Pid
	}
	if config.CpusetCpus != nil {
		hostConfig.CPUSet = *config.CpusetCpus
	}

	// Binds
	binds := []string{}
	for _, volume := range config.Volumes {
		if strings.Contains(volume, ":") {
			binds = append(binds, volume)
		}
	}
	if len(binds) > 0 {
		hostConfig.Binds = binds
	}

	// Privileged
	if config.Privileged != nil {
		hostConfig.Privileged = *config.Privileged
	}

	// PublishAllPorts
	if config.PublishAllPorts != nil {
		hostConfig.PublishAllPorts = *config.PublishAllPorts
	}

	// PortBindings
	if len(config.Ports) > 0 {
		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{}
		for _, configPort := range config.Ports {
			key := (docker.Port)(configPort.Port)
			binding := docker.PortBinding{configPort.HostIp, configPort.HostPort}
			if _, ok := hostConfig.PortBindings[key]; !ok {
				hostConfig.PortBindings[key] = []docker.PortBinding{}
			}
			hostConfig.PortBindings[key] = append(hostConfig.PortBindings[key], binding)
		}
	}

	// Links
	if len(config.Links) > 0 {
		hostConfig.Links = []string{}
		for _, link := range config.Links {
			hostConfig.Links = append(hostConfig.Links, link.String())
		}
	}

	// VolumesFrom
	if len(config.VolumesFrom) > 0 {
		hostConfig.VolumesFrom = []string{}
		for _, volume := range config.VolumesFrom {
			hostConfig.VolumesFrom = append(hostConfig.VolumesFrom, volume.String())
		}
	}

	// Ulimits
	if len(config.Ulimits) > 0 {
		hostConfig.Ulimits = []docker.ULimit{}
		for _, ulimit := range config.Ulimits {
			hostConfig.Ulimits = append(hostConfig.Ulimits, docker.ULimit{
				Name: ulimit.Name,
				Soft: ulimit.Soft,
				Hard: ulimit.Hard,
			})
		}
	}

	return hostConfig
}