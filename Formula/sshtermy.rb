# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.202"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.202/sshtermy_darwin_arm64"
      sha256 "1b5c8847560a22123790d4e835e16e78636937aa53237311b8fcaab63c107bd9"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.202/sshtermy_darwin_amd64"
      sha256 "fc1be7ea6ed4280768c8cba448a94ef9f96245ad6c275bc5d091015e86d569e4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.202/sshtermy_linux_arm64"
      sha256 "4476e47a416d55396f5d52c4099e927501c290391a51f2600fcc50d1e3d5ec45"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.202/sshtermy_linux_amd64"
      sha256 "ed3e4484f67c68ab47ce9d4208b2f66fbcb860fc99b7623af8c2668391442d78"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
