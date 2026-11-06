# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.133"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.133/sshtermy_darwin_arm64"
      sha256 "e240279bfc8f15b68d751bcea436a304836b7d818b7b442bf419dadfc8a5fecb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.133/sshtermy_darwin_amd64"
      sha256 "beef09dbd7f5b7c4bfbb388cfbef5ae7440a2f4353fb00beb4f14f3d0caca1dc"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.133/sshtermy_linux_arm64"
      sha256 "ca7fd45c4c9b8c8112ec2c392ddd0576965a80ea028b4c33d297322607e00bab"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.133/sshtermy_linux_amd64"
      sha256 "296d5eeb174bc2537a75b2126c0e7b2dab553917b09db2819c04b3bd0f0749ef"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
