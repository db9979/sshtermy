# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.271"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.271/sshtermy_darwin_arm64"
      sha256 "0cb89a2f981bebb9e47a59c0f44b419f8470982042934fd6510c4eb644736cd8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.271/sshtermy_darwin_amd64"
      sha256 "88e19cc2dd0e7415d3b287e77ade0d91b3fceaad943caaf3a311968663177e1a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.271/sshtermy_linux_arm64"
      sha256 "3d430128c122227f69c0b49f1241de2c5c7e7858b5bdc92a2c50800c0441d5f6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.271/sshtermy_linux_amd64"
      sha256 "d1888adf48c023d4d1ec28a7edc6b461e490dcef3aa56f4ab6694448fd70f77b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
