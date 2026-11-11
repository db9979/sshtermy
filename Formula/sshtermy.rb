# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.338"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.338/sshtermy_darwin_arm64"
      sha256 "6f88141d3ecf047a4dcca50c3348e1fc509fdf0d78f708c0facc8c28dc853794"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.338/sshtermy_darwin_amd64"
      sha256 "a85f2f351d3e0160a36f0e5fbc7f5a833136b9caab496b1481f61803fab559c3"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.338/sshtermy_linux_arm64"
      sha256 "b6ba0b11a0b2184e7c3079b72ec48067eea74a05707e28ea5fa3d5238c9a177d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.338/sshtermy_linux_amd64"
      sha256 "7ec163a8e6b9acc6da3afa8074d5870c969ac6159ff8fa7ea4029e42bc7f600f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
