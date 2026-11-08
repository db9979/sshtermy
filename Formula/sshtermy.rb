# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.246"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.246/sshtermy_darwin_arm64"
      sha256 "679dfa544917d2a3dc6136fc61948545b1eef0a5f31f7654eebf6035ee3d04f6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.246/sshtermy_darwin_amd64"
      sha256 "e40e55f23a801db1bd81c4a2b8dcecf7be9698f59de875308d66ce1e151723d2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.246/sshtermy_linux_arm64"
      sha256 "04781bbb72526f5a840d7f8869cd2c2f7d588a00b952f1a51003cf672ec5c435"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.246/sshtermy_linux_amd64"
      sha256 "a2a5a452f5894f9add1d66712f406fa0f3d7c1099d83ad34c11b23bee1cc24e7"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
