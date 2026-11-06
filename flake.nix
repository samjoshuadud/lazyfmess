{
  description = "Unofficial terminal client for Messenger";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      each = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = each (pkgs: {
        default = pkgs.mkShellNoCC {
          packages = with pkgs; [
            go_1_27
            gopls
            gotools
          ];
          env.GOTOOLCHAIN = "local";
        };
      });
    };
}
