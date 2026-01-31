{
  pkgs ? import <nixpkgs> { },
}:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    nodejs
    playwright-driver.browsers
  ];

  shellHook = ''
    export PLAYWRIGHT_BROWSERS_PATH=${pkgs.playwright-driver.browsers}
    export NIXOS=1
    export CRAWLEE_PURGE_ON_START=0
  '';
}

