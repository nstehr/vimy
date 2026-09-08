generator target {
    output_type "go"
    output_dir "../"
    version "0.219.0"
    on_generate "gofmt -w . && goimports -w ."
    client_package_name "github.com/nstehr/vimy/currie"
}
