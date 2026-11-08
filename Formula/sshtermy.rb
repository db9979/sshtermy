# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.269"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.269/sshtermy_darwin_arm64"
      sha256 "bf6af69c005c701d453939152a6d5ffb2d46b12208802a251568dce441755292"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.269/sshtermy_darwin_amd64"
      sha256 "ecc4f86993a599c188449acf03492ed19caa9c3e226850979e6cddfa3bb198d2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.269/sshtermy_linux_arm64"
      sha256 "a45214857f8ea91d468cb20ac7117bc5ef299198806102c1a8105fbdd5ab9b20"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.269/sshtermy_linux_amd64"
      sha256 "4e522cb49a623deac6a37dbe5482cac5ca459c639534c57e9b45c4be52ae9fce"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
