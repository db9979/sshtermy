# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.152"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.152/sshtermy_darwin_arm64"
      sha256 "2315aeaa4361b7f0bb20ced01aa80aa8328ad069ccb55114f94d610db84a9564"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.152/sshtermy_darwin_amd64"
      sha256 "9ecd82e219ffd1636acba5412746938d8dd09a5856a17f4d6c86ae22c0bd68bf"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.152/sshtermy_linux_arm64"
      sha256 "12e330088528db2e19a16347f436646b3715028208c4e082660c8c670d1b9758"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.152/sshtermy_linux_amd64"
      sha256 "64c071fa061572f8c54c2c8f982f5c3efc8ffbdd37fa9cf7b2071ada8e90ac1a"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
