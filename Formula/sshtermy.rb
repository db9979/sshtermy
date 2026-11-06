# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.149"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.149/sshtermy_darwin_arm64"
      sha256 "03e044d19049ca9ebbe3b4f1edab8d8352705d3230f19311727572423d6bedd7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.149/sshtermy_darwin_amd64"
      sha256 "51d3f1aa9250bc9f877db4a9285f9948efeb37666ff1032db633ef0a20b4075b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.149/sshtermy_linux_arm64"
      sha256 "c2fb0c07f9c3cac34699d261f5d19a6a403adf3530c7e752c97a5d82cdbafe61"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.149/sshtermy_linux_amd64"
      sha256 "6a0853c26516c0ee70e26219318258d11e708feaf323d9e54e0e499237a46ea9"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
