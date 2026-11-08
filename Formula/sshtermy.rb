# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.259"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.259/sshtermy_darwin_arm64"
      sha256 "fd5d7ea7be6f3a6e9c4d2a519d7c5e200622bde85f7e261f5b8ecdabc2299072"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.259/sshtermy_darwin_amd64"
      sha256 "7450b0ea735a55b532f870ea6fd056ff7bb065e59f8462758893eb7556dd68f0"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.259/sshtermy_linux_arm64"
      sha256 "2e6b083fefe5b047f034726eb6d626b30c046160f4d2bd6127f9b06ba51e0cc1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.259/sshtermy_linux_amd64"
      sha256 "1eefb4caaa73c37bb78a423a4d87dc6a5da3808fea7ef7f22b60536032ac8224"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
