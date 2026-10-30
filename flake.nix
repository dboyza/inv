# Build inv and provide a development shell that pins the Go toolchain.
{
  description = "inv: every custom install on your Mac, in one live terminal view";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      # inv reads macOS-only sources (app bundles, launchd, Spotlight), so only Darwin is supported.
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
      version = "0.1.0";
    in
    {
      # Section: Package
      packages = forAll (pkgs: rec {
        inv = pkgs.buildGoModule {
          pname = "inv";
          inherit version;
          src = nixpkgs.lib.cleanSource self;
          vendorHash = "sha256-rHmYHOroP1lJHDTk5Cc52ehcCt99gyQcT5QsrWJutEk=";
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
          ];
          meta = {
            description = "Every custom install on your Mac, in one live terminal view";
            homepage = "https://github.com/dboyza/inv";
            mainProgram = "inv";
            platforms = nixpkgs.lib.platforms.darwin;
          };
        };
        default = inv;
      });

      # Section: Development shell
      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
          ];
        };
      });
    };
}
