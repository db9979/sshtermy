# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.244"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.244/sshtermy_darwin_arm64"
      sha256 "b3850af7e2e484158f1531ac9bb89410999167d565f4a4f2ebe22994fb26339f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.244/sshtermy_darwin_amd64"
      sha256 "143ce31221ba4153f73f8c47d4fa0bec61bad3f0060845039e4f1b6ca1e11030"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.244/sshtermy_linux_arm64"
      sha256 "56ec64994556321443250f0734eb1965adab2cdffe12f085dbc58a5dadd5a52a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.244/sshtermy_linux_amd64"
      sha256 "2c7aa8e2f890202c40ac9992d0e49faf9e48f6666e90af66462655a2eb48e375"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
