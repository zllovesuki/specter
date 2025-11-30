package client

import (
	"context"
	"crypto/ed25519"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
)

func (c *Client) obtainAcmeProof(hostname string) (*protocol.ProofOfWork, error) {
	c.configMu.RLock()
	privKey, err := pki.UnmarshalPrivateKey([]byte(c.Configuration.PrivKey))
	c.configMu.RUnlock()
	if err != nil {
		return nil, err
	}

	return pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
}

func (c *Client) GetAcmeInstruction(ctx context.Context, hostname string) (*protocol.InstructionResponse, error) {
	proof, err := c.obtainAcmeProof(hostname)
	if err != nil {
		return nil, err
	}

	return retryRPC(c, ctx, func(node *protocol.Node) (*protocol.InstructionResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.AcmeInstruction(ctx, &protocol.InstructionRequest{
			Proof:    proof,
			Hostname: hostname,
		})
	})
}

func (c *Client) RequestAcmeValidation(ctx context.Context, hostname string) (*protocol.ValidateResponse, error) {
	proof, err := c.obtainAcmeProof(hostname)
	if err != nil {
		return nil, err
	}

	return retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ValidateResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.AcmeValidate(ctx, &protocol.ValidateRequest{
			Proof:    proof,
			Hostname: hostname,
		})
	})
}
