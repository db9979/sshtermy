# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.346"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.346/sshtermy_darwin_arm64"
      sha256 "1da804068a2f94600371359b250fa81a5ad076f72269057aea768e9fd8cf968c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.346/sshtermy_darwin_amd64"
      sha256 "9c43e270d3d1d488c14ece9f702992afe14b93ecc5f2f70e5b3e80ea0b205ceb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.346/sshtermy_linux_arm64"
      sha256 "b1c3567d37774537276425b9bf12b0058f2ec2cb653ed15e3e0e34e869ac14e8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.346/sshtermy_linux_amd64"
      sha256 "ec0ec8e6ddc16d7ce9f38b60421334f0db7f164e750ee5e832e648d7497bd39b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
