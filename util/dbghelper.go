package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// Command constants
	kubectlCmd = "kubectl"
	dockerCmd  = "docker"
	bashCmd    = "bash"
)

// Terminal colors
var (
	green  = "\033[0;32m"
	red    = "\033[0;31m"
	blue   = "\033[0;34m"
	grey   = "\033[0;37m"
	resetC = "\033[0m"
)

type KubeClient struct {
	clientset *kubernetes.Clientset
}

func newKubeClient() (*KubeClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	return &KubeClient{clientset: clientset}, nil
}

func (k *KubeClient) getAvailableNodeNames() ([]string, error) {
	nodes, err := k.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var nodeNames []string
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	return nodeNames, nil
}

func printColored(color, message string) {
	fmt.Printf("%s%s%s\n", color, message, resetC)
}

func handleError(err error, message string) {
	if err != nil {
		printColored(red, fmt.Sprintf("%s: %v", message, err))
		os.Exit(1)
	}
}

func kubectlCommand(args ...string) (string, error) {
	cmd := exec.Command(kubectlCmd, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func dockerCommand(args ...string) (string, error) {
	cmd := exec.Command(dockerCmd, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runInteractiveCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func validateNodeName(k *KubeClient, nodeName string) (string, error) {
	nodeNames, err := k.getAvailableNodeNames()
	if err != nil {
		return "", err
	}

	if len(nodeNames) == 0 {
		return "", fmt.Errorf("no nodes found. Is cluster running?")
	}

	if nodeName == "" && len(nodeNames) == 1 {
		return nodeNames[0], nil
	}

	for _, n := range nodeNames {
		if n == nodeName {
			return nodeName, nil
		}
	}

	var nodeList strings.Builder
	nodeList.WriteString("\nAvailable nodes:")
	for i, n := range nodeNames {
		nodeList.WriteString(fmt.Sprintf("\n%d. %s", i+1, n))
	}

	return "", fmt.Errorf("node '%s' not found.%s", nodeName, nodeList.String())
}

// Set A: kubectl related functions
func getNamespaces() error {
	printColored(blue, "Getting namespaces...")
	output, err := kubectlCommand("get", "namespaces")
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func getNodes() error {
	printColored(blue, "Getting nodes...")
	output, err := kubectlCommand("get", "nodes", "-o", "wide")
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func getPods() error {
	printColored(blue, "Getting pods...")
	output, err := kubectlCommand("get", "pods", "-A", "-o", "wide")
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// Set B: docker exec related functions
func getNodeShell(k *KubeClient, nodeName string) error {
	validatedNode, err := validateNodeName(k, nodeName)
	if err != nil {
		return err
	}

	printColored(grey, fmt.Sprintf("Getting bash shell for node '%s' ...", validatedNode))
	printColored(grey, fmt.Sprintf("Executing: docker exec -it --privileged %s bash", validatedNode))

	// Execute docker exec command
	err = runInteractiveCommand(dockerCmd, "exec", "-it", "--privileged", validatedNode, bashCmd)
	if err != nil {
		return fmt.Errorf("failed to exec into node container: %v", err)
	}

	return nil
}

// Set C: docker image/container related functions
func getImages() error {
	printColored(blue, "Getting Docker images...")
	output, err := dockerCommand("images")
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func getContainers() error {
	printColored(blue, "Getting Docker containers...")
	output, err := dockerCommand("ps", "-a")
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// DockerImage represents a Docker image with its details
type DockerImage struct {
	Repository string
	Tag        string
	ImageID    string
	Created    string
	Size       string
}

// parseDockerImages parses the output of 'docker images' and returns a slice of DockerImage
func parseDockerImages() ([]DockerImage, error) {
	output, err := dockerCommand("images", "--no-trunc", "--format", "table {{.Repository}}\\t{{.Tag}}\\t{{.ID}}\\t{{.CreatedAt}}\\t{{.Size}}")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("no images found")
	}

	var images []DockerImage
	// Skip header line
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			image := DockerImage{
				Repository: fields[0],
				Tag:        fields[1],
				ImageID:    fields[2],
				Created:    fields[3],
				Size:       fields[4],
			}
			images = append(images, image)
		}
	}

	return images, nil
}

// validateTag checks if the given tag exists in any Docker image
func validateTag(tag string) ([]DockerImage, error) {
	images, err := parseDockerImages()
	if err != nil {
		return nil, err
	}

	var matchingImages []DockerImage
	for _, img := range images {
		if img.Tag == tag {
			matchingImages = append(matchingImages, img)
		}
	}

	if len(matchingImages) == 0 {
		return nil, fmt.Errorf("no images found with tag '%s'", tag)
	}

	return matchingImages, nil
}

// validateRepo checks if the given repository exists in any Docker image
func validateRepo(repo string) ([]DockerImage, error) {
	images, err := parseDockerImages()
	if err != nil {
		return nil, err
	}

	var matchingImages []DockerImage
	for _, img := range images {
		if img.Repository == repo {
			matchingImages = append(matchingImages, img)
		}
	}

	if len(matchingImages) == 0 {
		return nil, fmt.Errorf("no images found with repository '%s'", repo)
	}

	return matchingImages, nil
}

// removeImagesByTag removes all Docker images with the specified tag
func removeImagesByTag(tag string) error {
	printColored(blue, fmt.Sprintf("Validating tag '%s'...", tag))
	matchingImages, err := validateTag(tag)
	if err != nil {
		return err
	}

	printColored(green, fmt.Sprintf("Found %d image(s) with tag '%s':", len(matchingImages), tag))
	for _, img := range matchingImages {
		fmt.Printf("  - %s:%s (%s)\n", img.Repository, img.Tag, img.ImageID[:12])
	}

	printColored(blue, "Removing images...")
	for _, img := range matchingImages {
		imageRef := fmt.Sprintf("%s:%s", img.Repository, img.Tag)
		printColored(grey, fmt.Sprintf("Removing %s...", imageRef))

		output, err := dockerCommand("rmi", imageRef)
		if err != nil {
			printColored(red, fmt.Sprintf("Failed to remove %s: %v", imageRef, err))
			if strings.TrimSpace(output) != "" {
				fmt.Printf("Output: %s\n", output)
			}
		} else {
			printColored(green, fmt.Sprintf("Successfully removed %s", imageRef))
		}
	}

	return nil
}

// removeImagesByRepo removes all Docker images with the specified repository
func removeImagesByRepo(repo string) error {
	printColored(blue, fmt.Sprintf("Validating repository '%s'...", repo))
	matchingImages, err := validateRepo(repo)
	if err != nil {
		return err
	}

	printColored(green, fmt.Sprintf("Found %d image(s) with repository '%s':", len(matchingImages), repo))
	for _, img := range matchingImages {
		fmt.Printf("  - %s:%s (%s)\n", img.Repository, img.Tag, img.ImageID[:12])
	}

	printColored(blue, "Removing images...")
	for _, img := range matchingImages {
		imageRef := fmt.Sprintf("%s:%s", img.Repository, img.Tag)
		printColored(grey, fmt.Sprintf("Removing %s...", imageRef))

		output, err := dockerCommand("rmi", imageRef)
		if err != nil {
			printColored(red, fmt.Sprintf("Failed to remove %s: %v", imageRef, err))
			if strings.TrimSpace(output) != "" {
				fmt.Printf("Output: %s\n", output)
			}
		} else {
			printColored(green, fmt.Sprintf("Successfully removed %s", imageRef))
		}
	}

	return nil
}

// handleImagesCommand handles the images command and its sub-commands
func handleImagesCommand(args []string) error {
	if len(args) == 0 {
		// Default behavior: show all images
		return getImages()
	}

	if len(args) < 1 {
		return fmt.Errorf("invalid images command")
	}

	switch args[0] {
	case "remove":
		if len(args) < 3 {
			return fmt.Errorf("usage: dbghelper images remove --tag <TAG> or dbghelper images remove --repo <REPO>")
		}

		flag := args[1]
		value := args[2]

		switch flag {
		case "--tag":
			return removeImagesByTag(value)
		case "--repo":
			return removeImagesByRepo(value)
		default:
			return fmt.Errorf("unknown flag '%s'. Use --tag or --repo", flag)
		}

	default:
		return fmt.Errorf("unknown images sub-command '%s'", args[0])
	}
}

func printHelp() {
	banner := `     _ _           _          _                 
  __| | |__   __ _| |__   ___| |_ __   ___ _ __ 
 / _` + "`" + ` | '_ \ / _` + "`" + ` | '_ \ / _ \ | '_ \ / _ \ '__|
| (_| | |_) | (_| | | | |  __/ | |_) |  __/ |   
 \__,_|_.__/ \__, |_| |_|\___|_| .__/ \___|_|   
             |___/             |_|              

Debug Helper Tool
`
	fmt.Print(banner)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println()
	fmt.Println("dbghelper namespaces                    - Get namespaces (kubectl get namespaces)")
	fmt.Println("dbghelper nodes                         - Get nodes (kubectl get nodes -o wide)")
	fmt.Println("dbghelper pods                          - Get pods (kubectl get pods -A -o wide)")
	fmt.Println()
	fmt.Println("dbghelper sh <NODE>                     - Get bash shell in node (docker exec -it --privileged <NODE> bash)")
	fmt.Println()
	fmt.Println("dbghelper images                        - Get Docker images (docker images)")
	fmt.Println("dbghelper images remove --tag <TAG>     - Remove all images with specified tag")
	fmt.Println("dbghelper images remove --repo <REPO>   - Remove all images with specified repository")
	fmt.Println("dbghelper containers                    - Get Docker containers (docker ps -a)")
	fmt.Println()
	fmt.Println("dbghelper all                           - Run all kubectl commands (namespaces, nodes, pods)")
	fmt.Println("dbghelper help                          - Show this help message")
}

func runAllKubectlCommands() error {
	if err := getNamespaces(); err != nil {
		return err
	}
	fmt.Println()

	if err := getNodes(); err != nil {
		return err
	}
	fmt.Println()

	if err := getPods(); err != nil {
		return err
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	// Commands that don't require Kubernetes client
	switch command {
	case "help", "-h", "--help":
		printHelp()
		return
	case "images":
		err := handleImagesCommand(args)
		if err != nil {
			handleError(err, "Failed to handle images command")
		}
		return
	case "containers":
		err := getContainers()
		if err != nil {
			handleError(err, "Failed to get Docker containers")
		}
		return
	}

	// Commands that require Kubernetes client
	k, err := newKubeClient()
	if err != nil {
		handleError(err, "Failed to create Kubernetes client")
	}

	switch command {
	case "namespaces", "ns":
		err := getNamespaces()
		if err != nil {
			handleError(err, "Failed to get namespaces")
		}

	case "nodes":
		err := getNodes()
		if err != nil {
			handleError(err, "Failed to get nodes")
		}

	case "pods":
		err := getPods()
		if err != nil {
			handleError(err, "Failed to get pods")
		}

	case "sh":
		if len(args) < 1 {
			handleError(fmt.Errorf("missing node name"), "Node name is required for 'sh' command")
		}
		nodeName := args[0]
		err := getNodeShell(k, nodeName)
		if err != nil {
			handleError(err, "Failed to get node shell")
		}

	case "all":
		err := runAllKubectlCommands()
		if err != nil {
			handleError(err, "Failed to run all kubectl commands")
		}

	default:
		printColored(red, fmt.Sprintf("Unknown command: %s", command))
		printHelp()
		os.Exit(1)
	}
}
