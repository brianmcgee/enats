{
  perSystem = {
    lib,
    pkgs,
    ...
  }: {
    packages = rec {
      enats = pkgs.buildGoModule rec {
        pname = "enats";
        version = "0.0.1+dev";

        src = ../.;
        vendorSha256 = "sha256-JVt3OzYkeBGjNDjAYkZkLyB8fUyJsdNnQ/2H3bVBL+s=";

        ldflags = [
          "-X 'build.Name=${pname}'"
          "-X 'build.Version=${version}'"
        ];

        meta = with lib; {
          description = "NATS Eth Sidecar for relaying json rpc requests.";
          homepage = "https://github.com/brianmcgee/enats";
          license = licenses.apsl20;
        };
      };

      default = enats;
    };
  };
}
