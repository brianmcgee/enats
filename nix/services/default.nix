{
  inputs,
  lib,
  ...
}: {
  perSystem = {
    system,
    self',
    pkgs,
    ...
  }: let
    # todo is there a better way than having to create a new instance of nixpkgs
    foundry = inputs.foundry.defaultPackage.${system};
  in {
    config.process-compose.configs = {
      dev-services.processes = let
        config = ./nats.conf;
      in {
        nats.command = "${lib.getExe pkgs.nats-server} -c ${config}";
        anvil.command = "${foundry}/bin/anvil --block-time 1";
      };
    };

    config.devshells.default = {
      commands = let
        category = "development";
      in [
        {
          inherit category;
          help = "run local dev services";
          package = self'.packages.dev-services;
        }
        {
          inherit category;
          name = "cast";
          help = "perform Ethereum RPC calls";
          command = "${foundry}/bin/cast $@";
        }
      ];
    };
  };
}
