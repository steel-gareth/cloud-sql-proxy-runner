{
  description = "cloud-proxy - manage Cloud SQL proxy connections from a single YAML config";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule rec {
          pname = "cloud-proxy";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-iEO3FHEMeqXidNyP6XnMO0f54PWn0jemJAm1moSofd4=";
          ldflags = [
            "-X cloud-proxy/cmd.version=${version}"
            "-X cloud-proxy/cmd.gitCommit=${self.shortRev or "dirty"}"
            "-X cloud-proxy/cmd.buildTime=${self.lastModifiedDate or "unknown"}"
          ];
          meta = {
            description = "Start/stop/list Cloud SQL proxy connections from a single YAML config";
            mainProgram = "cloud-proxy";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.gopls ];
        };
      }
    );
}
