{
  description = "Basic go flake";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ ];
        };
      in
      {
        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [
            go-task
            go
            gopls
            gotools
            go-outline
            gopkgs
            godef
            golint
          ];

          shellHook = ''
            echo "Golang development environment loaded"
            export GOPATH=$HOME/go
            export PATH=$GOPATH/bin:$PATH
          '';
        };
      }
    );
}
