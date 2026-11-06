# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.186"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.186/sshtermy_darwin_arm64"
      sha256 "c41baadf75cf883d70504a0fd8460a1a748f8eade0182d8660778e963702f76f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.186/sshtermy_darwin_amd64"
      sha256 "c5fe05f4827fb26d97ad6b203f1e3999bfdc1bcf2e0891cc9da43a288311cc07"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.186/sshtermy_linux_arm64"
      sha256 "e7283098afb1b1bffef8587f3d974117bdcab06e6455814985f90c0c86ac7e26"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.186/sshtermy_linux_amd64"
      sha256 "5262c3e77f0bbec90cba69744a2cecee1bb950d75a11b5092a996b0db0abec79"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
